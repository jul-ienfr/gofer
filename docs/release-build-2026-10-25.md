# Release build verification — 2026-10-25

Status: local cross-build of all six target binaries with embedded assets. The
final Windows amd64 build ran natively and was audited. The final Linux amd64
source was built and audited as a Docker image; an earlier Linux binary also ran
under WSL2 Ubuntu 22.04. The binaries were not signed, notarized, or published,
and macOS and Windows arm64 were not launched.

## Command

```powershell
.\scripts\build-release.ps1 `
  -OutputDir "$env:TEMP\gofer-release-final3" `
  -Version phase8-final3 `
  -Templ "$(go env GOPATH)\bin\templ.exe"
```

The script ran `templ generate`, Tailwind 4.3.3, esbuild 0.28.2, and
`go build -tags embedded_assets -trimpath`. This build includes the
newer-schema startup guard added by the rollback drill.

## Artifacts

| Target | Size |
| --- | --- |
| windows/amd64 | 68,676,096 bytes |
| windows/arm64 | 61,988,352 bytes |
| linux/amd64 | 68,333,147 bytes |
| linux/arm64 | 62,178,008 bytes |
| darwin/amd64 | 69,204,128 bytes |
| darwin/arm64 | 63,639,378 bytes |

`SHA256SUMS` was generated and the Windows amd64 entry matched a fresh
`Get-FileHash` calculation.

```text
4f008cdb3cf29a52e7c99749be911c67641f668f9f6cabc2f42701e72b235187  gofer-darwin-amd64
cebe2a8079b46464ad916cfe9c1f6100799afe24e79b72ddbc42e055fa405794  gofer-darwin-arm64
55772ba0215207f4bcabb05490c616974e22ab562d43be819192e8f10a6dde10  gofer-linux-amd64
59fc4d9191015d351e3471d8477d053f1910725da0784c0c511ea585d97897d9  gofer-linux-arm64
ef0a8a4a771503dd342e4942e252e94fe34ff6fc1c371933f05631e63286c519  gofer-windows-amd64.exe
29ad4f2237efa1d44262ac1deca866a27772051fc1f4196e12191fe792adcdd3  gofer-windows-arm64.exe
```

## Smoke test of the embedded Windows build

- Started on a fresh open-mode database.
- `/settings/about` showed version `phase8-final3`.
- `/healthz` and `/readyz` returned 200.
- `scripts/a11y-keyboard.cjs`: 10 checks passed.
- `scripts/a11y-reflow.cjs`: 17 checks passed at 320 px, plus 17 with text
  spacing overrides.
- `scripts/a11y-motion.cjs`: 7 reduced-motion checks passed.
- `scripts/a11y-themes.cjs`: 136 checks passed across all four looks, both
  color modes, and all 17 pages.

## Smoke test of the Linux build

An earlier Linux amd64 binary with the same embedded assets ran under WSL2
Ubuntu 22.04: `/healthz`, `/readyz`, and `/` returned 200; the keyboard script
passed 10 checks; axe reported zero violations on `/`, `/contacts`, and
`/calendar`.

The final source was also built as a Docker image and audited there. WSL
localhost forwarding stopped working after a WSL restart, so the final Linux
binary was not re-run under WSL.

## Docker image

The first image build failed because the Tailwind CLI checksums in the
`Dockerfile` did not match the v4.3.3 musl assets. The correct SHA-256 values
are now pinned for both architectures. After the fix:

```powershell
docker build --build-arg VERSION=phase8-final3 --build-arg COMMIT=local `
  -t gofer:phase8-final3 .
```

The final build completed, producing a 77.7 MB image. The container ran with
`GOFER_AUTH_MODE=open` and a disposable local CIDR allowance; Docker reported it
healthy. `/healthz` and `/readyz` returned 200, `/settings/about` showed
`phase8-final3`, the keyboard script passed 10 checks, the reflow script passed
with text spacing, reduced motion passed, and all 136 theme audits passed.

The open-mode guard correctly refused the first container start because
`GOFER_ADDR=0.0.0.0:8090` with a loopback base URL and no allowed CIDRs would
have exposed an unauthenticated service. The test override
`GOFER_ALLOWED_CIDRS=172.16.0.0/12` is only for this disposable local container.

## Helm chart

The chart was validated with the same commands as the Helm CI workflow, using
Helm 3.16.4 and kubeconform 0.8.0 containers:

```text
helm lint --strict: 1 chart linted, 0 failed
helm template for default, existing-secret, httproute, and ingress values:
  28 resources
kubeconform -strict -summary -ignore-missing-schemas -kubernetes-version 1.31.0:
  Valid: 27, Invalid: 0, Errors: 0, Skipped: 1
helm package: gofer-0.1.4.tgz (6,088 bytes)
```

The skipped resource is the Gateway API HTTPRoute, whose CRD schema is not in
the default kubeconform catalog.

`docker compose config --quiet` also accepted the repository's `compose.yaml`.

## Gaps

- No code signing, Authenticode timestamp, Apple notarization, or package
  installers. The Docker image is unsigned and was not pushed to a registry.
- No bare-metal Linux, macOS, or Windows arm64 execution. Linux amd64 ran under
  WSL2, which shares the host kernel and localhost forwarding.
- No staged release or upgrade/rollback from these exact artifacts. The local
  migration rollback drill used separately built N and N+1 binaries; see
  [release-rollback.md](./release-rollback.md).
