# Release build verification — 2026-10-25

Status: local cross-build of all six target binaries with embedded assets. The
Windows amd64 build ran natively and the Linux amd64 build ran under WSL2
Ubuntu 22.04. The binaries were not signed, notarized, or published, and macOS
and Windows arm64 were not launched.

## Command

```powershell
.\scripts\build-release.ps1 `
  -OutputDir "$env:TEMP\gofer-release-check" `
  -Version phase8-check `
  -Templ "$(go env GOPATH)\bin\templ.exe"
```

The script ran `templ generate`, Tailwind 4.3.3, esbuild 0.28.2, and
`go build -tags embedded_assets -trimpath`.

## Artifacts

| Target | Size |
| --- | --- |
| windows/amd64 | 68,641,792 bytes |
| windows/arm64 | 61,953,024 bytes |
| linux/amd64 | 68,300,515 bytes |
| linux/arm64 | 62,176,272 bytes |
| darwin/amd64 | 69,171,360 bytes |
| darwin/arm64 | 63,606,354 bytes |

`SHA256SUMS` was generated and the Windows amd64 entry matched a fresh
`Get-FileHash` calculation.

```text
ab401ed89527957efd7c3658860392cc33ed53688dc1eb93a632537d0ba7931a  gofer-darwin-amd64
073dbd737651546924f0038b166c77350c40ead95f0cb2c2c266331fe04780e1  gofer-darwin-arm64
cf34917cf63c42013144694d2fdf5976a953283c1a780ce680bc7a3412356f46  gofer-linux-amd64
23ae1aa9a957ede27ed08b5be00d8cf151cbcd083c75b3bf3bd0d85de8d6e9d9  gofer-linux-arm64
93823ba245e8049447bead15c296c6e427536020d2a13f4189f752e3e70d1870  gofer-windows-amd64.exe
86f15412552a67a23448b34e67571c7a158fba92bcdcb28f85550862d60fe01b  gofer-windows-arm64.exe
```

## Smoke test of the embedded Windows build

- Started on a fresh open-mode database.
- `/settings/about` showed version `phase8-check`.
- `scripts/a11y-keyboard.cjs`: 10 checks passed.
- `scripts/a11y-browser.cjs`: zero violations on `/`, `/contacts`, and
  `/calendar`.

## Smoke test of the embedded Linux build

The Linux amd64 binary was rebuilt with the same embedded assets and run under
WSL2 Ubuntu 22.04 on the same host, on a fresh open-mode database.

- `/healthz` and `/readyz` returned 200.
- `/` returned 200.
- `scripts/a11y-keyboard.cjs`: 10 checks passed against the Linux-served page.
- `scripts/a11y-browser.cjs`: zero violations on `/`, `/contacts`, and
  `/calendar`.

## Docker image

The first image build failed because the Tailwind CLI checksums in the
`Dockerfile` did not match the v4.3.3 musl assets. The correct SHA-256 values
are now pinned for both architectures. After the fix:

```powershell
docker build --build-arg VERSION=phase8-check --build-arg COMMIT=local `
  -t gofer:phase8-check .
```

The build completed, producing a 77.6 MB image. The container ran with
`GOFER_AUTH_MODE=open` and a disposable local CIDR allowance; Docker reported it
healthy. `/settings/about` showed `phase8-check`, the keyboard script passed 10
checks, and axe reported zero violations on `/`, `/contacts`, `/calendar`, and
`/settings/about`.

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
- No staged release, upgrade test, or rollback test from these artifacts.
