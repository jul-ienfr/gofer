# Phase 8 hardening evidence

This file maps the Phase 8 work to repository evidence. It distinguishes what is implemented and reproducible from what still needs an independent or production-sized run.

## Status summary

| Gate G8 item | Status | Evidence |
| --- | --- | --- |
| Threat model and ASVS mapping | Implemented; independent review pending | [threat-model.md](./threat-model.md) |
| Penetration test | Not performed | No external test has been commissioned. Automated coverage is listed in the threat model. |
| Backup and restore | Implemented and tested | [backup.go](../internal/backup/backup.go), [backup tests](../internal/backup/backup_hardening_test.go), [CLI tests](../internal/server/storage_backup_command_test.go), [runbook](./backup-restore.md) |
| S2/S3 load test | Seeded S2 pass and 100-VU stress pass on one host; production-sized S3 and maximum-capacity result pending | [perf/load.js](../perf/load.js), [perf/README.md](../perf/README.md), [seeded S2 evidence](../perf/evidence-2026-10-25-seeded.md), [stress evidence](../perf/evidence-2026-10-25-stress.md) |
| Chaos: Jev/LLM/webhook/DB | DB crash/corruption drill passed; LLM, Jev, and webhook failures covered by HTTP fault tests; live end-to-end provider injection pending | [chaos evidence](../perf/evidence-2026-10-25-chaos.md), [perf/README.md](../perf/README.md), `internal/jev`, `internal/llm`, `internal/routing`, `internal/aiworker` tests |
| RPO 15 min / RTO 1 h | Targets documented; local 35.7 MB restore drill passed, production drill pending | [backup-restore.md](./backup-restore.md) |
| WCAG 2.2 AA | Structural, axe, keyboard/focus, and 320 px reflow automation pass on empty and seeded data; manual assistive-technology audit pending | [accessibility-i18n.md](./accessibility-i18n.md), [a11ycheck](../cmd/a11ycheck/main.go), [a11y-browser.cjs](../scripts/a11y-browser.cjs), [a11y-keyboard.cjs](../scripts/a11y-keyboard.cjs), [a11y-reflow.cjs](../scripts/a11y-reflow.cjs), [a11y-motion.cjs](../scripts/a11y-motion.cjs), [a11y-themes.cjs](../scripts/a11y-themes.cjs) |
| i18n | French complete; other catalogs in progress | JSON catalogs for English, French, Spanish, Brazilian Portuguese, and Simplified Chinese under `internal/i18n/locales`; a per-user interface language stored as `language` and a `gofer_lang` cookie for signed-out pages; template strings use `i18n.T`/`i18n.N` with English fallback. The French catalog is fully translated, including the fork AI templates and client scripts, with only brand names and identical terms left in English. Dates, times, numbers, and AI status labels follow the language settings. RTL and human translation review remain. See [accessibility-i18n.md](./accessibility-i18n.md). |
| Three-OS packaging | Implemented; six-target archives and checksums built by the release workflow on `v*` tags, with Windows, Linux/WSL2, and Docker audits verified | [release packaging](./release-packaging.md); [release build evidence](./release-build-2026-10-25.md); [.github/workflows/release.yml](../.github/workflows/release.yml); [release-package.sh](../scripts/release-package.sh) |
| Runbooks | Implemented | [backup-restore.md](./backup-restore.md), [release-rollback.md](./release-rollback.md) |
| Rollback tested | Local migration rollback drill passed; production drill pending | [release-rollback.md](./release-rollback.md) |

## Upstream i18n merge (2026-11-09)

The fork merged 117 upstream commits as `9ef68f86`. Upstream now ships JSON catalogs for English, Spanish, Brazilian Portuguese, and Simplified Chinese, and the fork adds French in `internal/i18n/locales/fr.json`. The merged schema is version 153: upstream migrations 136–143 run first, then the fork AI migrations 144–153. Legacy fork databases at version 145 are detected, rewound to 135 so the upstream migrations can run, and then marked 153 without rerunning the AI migrations. `go build ./...`, `go vet ./...`, and `go test ./...` pass after the merge; the French catalog was completed afterwards, and remaining follow-ups are tracked in [accessibility-i18n.md](./accessibility-i18n.md).

## Upstream v0.20.0 merge (2026-11-02)

The fork merged `upstream/master` as commit `7f258bc0`. The base schema now comes from upstream at version 135; the fork AI migrations are 136–145 and `CurrentSchemaVersion` is 145. A fresh-database smoke run created schema 145, served `/healthz` and `/login` with status 200, and `go build ./...`, `go vet ./...` and `go test ./... -count=1` pass. `govulncheck` v1.9.0 reports 0 vulnerabilities affecting the code. Pre-merge fork databases at schema 144 are not upgraded automatically because their AI migration numbers collide with the new upstream numbering; recreate them.

## Reproducible checks

```sh
gofmt -l internal/a11y internal/backup cmd/a11ycheck main.go
go vet ./...
go test ./internal/backup ./internal/a11y ./internal/server -run 'TestStorage|TestCreateVerify|TestRestore|TestAudit' -count=1
go build ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
docker build --build-arg VERSION=ci -t gofer:ci .
docker run --rm -v "$PWD:/repo" -w /repo rhysd/actionlint:1.7.7 -color
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
GOFER_BASE_URL=http://127.0.0.1:8090 node scripts/a11y-reflow.cjs
GOFER_BASE_URL=http://127.0.0.1:8090 node scripts/a11y-motion.cjs
GOFER_BASE_URL=http://127.0.0.1:8090 node scripts/a11y-themes.cjs
```

A local open-mode run with an empty database returned zero structural and browser-audit violations on 2026-10-25 for `/`, `/contacts`, `/calendar`, all 13 settings pages, and `/setup` at 1280×900 and 390×844, in light and dark color schemes. The seeded 10,000-message database also returned zero axe violations on `/`, `/contacts`, and `/calendar` at desktop and mobile, and the 10-check keyboard script passed on both databases. This does not replace the manual checklist.

A local backup and restore drill on the same date used a 1.5 MB, 6-file data directory at schema version 142: backup took 0.16 s, verification passed, and restore to a scratch directory took 0.03 s. A second drill used a 35.7 MB seeded database: a 4.27 MB archive, 0.40 s backup, 0.12 s restore, and a restored server with 10,000 messages, 1,000 contacts, 500 events, and a clean integrity check. These prove the command path and local speed, not the one-hour RTO on production storage.

A local release build on 2026-10-25 produced all six targets (Windows, Linux, and macOS on amd64 and arm64) plus `SHA256SUMS`; the Windows amd64 checksum matched. After the accessibility fixes the build was repeated as `phase8-check`, then as `phase8-final` after the rollback guard, and again as `phase8-final3` after the reduced-motion and theme-contrast fixes. The final Windows binary passed health, keyboard, reflow, reduced-motion, and 136 theme-audit checks, and Settings > About showed `phase8-final3`; the final source also built and passed the same checks as a 77.7 MB Docker image. An earlier Linux amd64 binary passed health, keyboard, and axe checks under WSL2 Ubuntu 22.04; WSL localhost forwarding stopped working before the final Linux binary could be re-run there. The Helm chart passed strict lint, rendered four value sets, validated 27 resources with kubeconform (one Gateway API CRD skipped), and packaged successfully. Details and hashes are in [the release build evidence](./release-build-2026-10-25.md). The Gofer binary does not expose a `version` subcommand.

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
- CI now builds the Docker image on pull requests without pushing it, so a broken Dockerfile or stale asset checksum fails before a release tag.
- CI now runs `actionlint` 1.7.7 on every pull request. It passes all workflows; it also found and we removed an unused shell variable in the accessibility job.
- Added an LLM provider-timeout regression test alongside the existing webhook and Jev HTTP fault tests.
- Added a 320 CSS-pixel reflow audit for the mail, contacts, Calendar, and all settings pages; 17 pages pass without horizontal document scrolling, including with WCAG 1.4.12 text-spacing overrides.
- Added a reduced-motion audit; it found fade-in and pulse utilities still running under `prefers-reduced-motion`, now disabled with the slide utilities.
- Added a theme audit across all four looks, both color modes, and all 17 primary and settings pages; it found and we fixed the Express select-all target size, Express link and destructive-button contrast, the Calendar "Today" badge contrast, and two amber warning contrasts. All 136 checks now pass.
- The rollback drill found that a binary could start against a newer schema. `storage.New` now rejects any schema version above the supported version before writing, with a regression test.
- The folder-thread schema test now normalizes CRLF so the Windows checkout does not fail on line endings alone.
- The fork's default branch `master` now triggers CI, Security, Accessibility, and Windows, and all four pass. The first runs exposed and we fixed 8.3 short-path handling in migration inspection, the contacts active-filter ARIA role, and a PowerShell `$home` conflict in the Windows smoke script.

## Residual gaps

These are real gaps, not documentation placeholders:

1. No independent third-party penetration test.
2. No production-sized or maximum-capacity S3 run. The seeded S2 and 100-VU stress results are recorded in [the seeded evidence](../perf/evidence-2026-10-25-seeded.md) and [the stress evidence](../perf/evidence-2026-10-25-stress.md); they cover one Windows host and do not exercise writes or provider sync.
3. No recorded live Jev/LLM/webhook/provider fault drill. A process-kill, restart, and corrupt-database drill is recorded in [the chaos evidence](../perf/evidence-2026-10-25-chaos.md).
4. No recorded RPO/RTO restore drill with production data. The local 35.7 MB drill is recorded in [backup-restore.md](./backup-restore.md).
5. No manual screen-reader or assistive-technology audit. The keyboard script covers the skip link, Compose focus trap and return, and positive tabindex, but not announcements, full page focus order, or real assistive technology.
6. No localization or RTL support.
7. Restore overwrites listed files but does not delete unrelated files already in the target directory.
8. No production upgrade/rollback drill or Helm/Docker rollback. The local same-host migration rollback drill is recorded in [release-rollback.md](./release-rollback.md).

Gate G8 should remain open until items 1 through 4 have recorded evidence, and the accessibility claim should remain "targeting WCAG 2.2 AA" until item 5 is complete. Item 8 also needs production evidence.
