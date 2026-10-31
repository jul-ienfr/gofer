# Phase 8 hardening evidence

This file maps the Phase 8 work to repository evidence. It distinguishes what is implemented and reproducible from what still needs an independent or production-sized run.

## Status summary

| Gate G8 item | Status | Evidence |
| --- | --- | --- |
| Threat model and ASVS mapping | Implemented; independent review pending | [threat-model.md](./threat-model.md) |
| Penetration test | Not performed | No external test has been commissioned. Automated coverage is listed in the threat model. |
| Backup and restore | Implemented and tested | [backup.go](../internal/backup/backup.go), [backup tests](../internal/backup/backup_hardening_test.go), [CLI tests](../internal/server/storage_backup_command_test.go), [runbook](./backup-restore.md) |
| S2/S3 load test | Seeded S2 pass and 100-VU stress pass on one host; production-sized S3 and maximum-capacity result pending | [perf/load.js](../perf/load.js), [perf/README.md](../perf/README.md), [seeded S2 evidence](../perf/evidence-2026-10-25-seeded.md), [stress evidence](../perf/evidence-2026-10-25-stress.md) |
| Chaos: Jev/LLM/webhook/DB | DB crash/corruption drill passed; Jev/LLM/webhook/live-provider injection pending | [chaos evidence](../perf/evidence-2026-10-25-chaos.md), [perf/README.md](../perf/README.md), `internal/jev`, `internal/llm`, `internal/routing`, `internal/aiworker` tests |
| RPO 15 min / RTO 1 h | Targets documented; local 35.7 MB restore drill passed, production drill pending | [backup-restore.md](./backup-restore.md) |
| WCAG 2.2 AA | Structural, axe, and keyboard/focus automation pass on empty and seeded data; manual assistive-technology audit pending | [accessibility-i18n.md](./accessibility-i18n.md), [a11ycheck](../cmd/a11ycheck/main.go), [a11y-browser.cjs](../scripts/a11y-browser.cjs), [a11y-keyboard.cjs](../scripts/a11y-keyboard.cjs) |
| i18n | Not implemented | English-only interface documented in [accessibility-i18n.md](./accessibility-i18n.md). |
| Three-OS packaging | Implemented; six-target build, checksums, Windows, Linux/WSL2, and Docker audits verified | [release build evidence](./release-build-2026-10-25.md); CI matrix in [.github/workflows/ci.yml](../.github/workflows/ci.yml); [build-release.ps1](../scripts/build-release.ps1); [release-build.sh](../scripts/release-build.sh) |
| Runbooks | Implemented | [backup-restore.md](./backup-restore.md), [release-rollback.md](./release-rollback.md) |
| Rollback tested | Procedure documented; no production drill recorded | [release-rollback.md](./release-rollback.md) |

## Reproducible checks

```sh
gofmt -l internal/a11y internal/backup cmd/a11ycheck main.go
go vet ./...
go test ./internal/backup ./internal/a11y ./internal/server -run 'TestStorage|TestCreateVerify|TestRestore|TestAudit' -count=1
go build ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```

Backup and restore end to end:

```sh
go test ./internal/backup -run TestCreateVerifyRestore -count=1
go test ./internal/server -run 'TestStorageBackupAndRestoreCommands|TestStorageRestoreCommandRequiresAllowNewerSchema|TestStorageBackupCommandHelpAndUsage' -count=1
```

Accessibility against a running instance:

```sh
go run ./cmd/a11ycheck -base http://127.0.0.1:8090 \
  -path / -path /contacts -path /calendar -path /settings

npm install --no-save --no-package-lock \
  @tailwindcss/cli@4.3.3 playwright@1.64.1 @axe-core/playwright@4.11.0 axe-core@4.11.0
npx playwright install chromium
task generate
node scripts/a11y-browser.cjs --base http://127.0.0.1:8090 \
  --path / --path /contacts --path /calendar --path /settings
GOFER_BASE_URL=http://127.0.0.1:8090 node scripts/a11y-keyboard.cjs
```

A local open-mode run with an empty database returned zero structural and browser-audit violations on 2026-10-25 for `/`, `/contacts`, `/calendar`, all 13 settings pages, and `/setup` at 1280×900 and 390×844, in light and dark color schemes. The seeded 10,000-message database also returned zero axe violations on `/`, `/contacts`, and `/calendar` at desktop and mobile, and the 10-check keyboard script passed on both databases. This does not replace the manual checklist.

A local backup and restore drill on the same date used a 1.5 MB, 6-file data directory at schema version 142: backup took 0.16 s, verification passed, and restore to a scratch directory took 0.03 s. A second drill used a 35.7 MB seeded database: a 4.27 MB archive, 0.40 s backup, 0.12 s restore, and a restored server with 10,000 messages, 1,000 contacts, 500 events, and a clean integrity check. These prove the command path and local speed, not the one-hour RTO on production storage.

A local release build on 2026-10-25 produced all six targets (Windows, Linux, and macOS on amd64 and arm64) plus `SHA256SUMS`; the Windows amd64 checksum matched. After the accessibility fixes, the build was repeated with version `phase8-check`; the embedded Windows binary passed the keyboard and axe checks, and Settings > About showed the expected version. The embedded Linux amd64 binary was rebuilt and run under WSL2 Ubuntu 22.04; health, keyboard, and axe checks passed there too. A Docker image also built after correcting the Tailwind v4.3.3 checksums, reported healthy, and passed the same checks. The Helm chart passed strict lint, rendered four value sets, validated 27 resources with kubeconform (one Gateway API CRD skipped), and packaged successfully. Details and hashes are in [the release build evidence](./release-build-2026-10-25.md). The Gofer binary does not expose a `version` subcommand.

## Security hardening added in this phase

- Restore rejects absolute paths, backslashes, `..`, symlinks, hardlinks, devices, duplicate entries, duplicate manifests, unlisted files, and unsafe database paths.
- Restore limits the manifest to 10 MiB, entries to 1,000,000, and total extraction to 100 GiB, enforcing the size cap while copying.
- Restore writes a safety archive before changing an existing installation.
- `gofer storage verify` checks the complete archive and every checksum.
- Backup skips WAL/SHM/journal files, earlier archives, sockets, and its own output.
- The CLI exposes backup, verify, and restore with explicit force and schema controls.
- CI now runs the backup and accessibility packages on Linux, macOS, and Windows.
- A dedicated workflow runs the axe browser audit on desktop, dark, and mobile viewports in [accessibility.yml](../.github/workflows/accessibility.yml).
- `govulncheck` found 19 reachable findings in the old `x/net` and Go standard library; upgrades to Go 1.27.3 and `x/net` v0.62.0 cleared them. A weekly and pull-request gate is in [security.yml](../.github/workflows/security.yml).
- The browser audit found and we fixed unnamed recipient fields, filter ARIA state, target sizes including seeded month events, and a color-only link.
- The keyboard audit found and we fixed the mail shortcut swallowing Enter on focused links and buttons, the skip link not moving `activeElement`, and the Compose button staying disabled after its dialog closed.
- The Docker build now pins the correct Tailwind v4.3.3 musl checksums. The open-mode guard refused an unauthenticated `0.0.0.0` container start until a CIDR allowance was supplied for the disposable test.

## Residual gaps

These are real gaps, not documentation placeholders:

1. No independent third-party penetration test.
2. No production-sized or maximum-capacity S3 run. The seeded S2 and 100-VU stress results are recorded in [the seeded evidence](../perf/evidence-2026-10-25-seeded.md) and [the stress evidence](../perf/evidence-2026-10-25-stress.md); they cover one Windows host and do not exercise writes or provider sync.
3. No recorded live Jev/LLM/webhook/provider fault drill. A process-kill, restart, and corrupt-database drill is recorded in [the chaos evidence](../perf/evidence-2026-10-25-chaos.md).
4. No recorded RPO/RTO restore drill with production data. The local 35.7 MB drill is recorded in [backup-restore.md](./backup-restore.md).
5. No manual screen-reader or assistive-technology audit. The keyboard script covers the skip link, Compose focus trap and return, and positive tabindex, but not announcements, full page focus order, or real assistive technology.
6. No localization or RTL support.
7. Restore overwrites listed files but does not delete unrelated files already in the target directory.

Gate G8 should remain open until items 1 through 4 have recorded evidence, and the accessibility claim should remain "targeting WCAG 2.2 AA" until item 5 is complete.
