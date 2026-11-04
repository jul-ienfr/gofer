# Release packaging

The **Release packages** workflow builds Gofer's six cross-platform archives and
publishes them with a GitHub release.

## Workflow

[.github/workflows/release.yml](../.github/workflows/release.yml) runs on a
`v*` tag and on manual dispatch:

- Tag: builds the archives, verifies the attribution files, uploads the
  workflow artifact, verifies the checksums, and creates or updates the GitHub
  release for the tag.
- Manual dispatch: builds the same archives and uploads them as a workflow
  artifact. It does not publish a release. The `version` input defaults to
  `dev`.

The build runs on Ubuntu with Go 1.27, templ v0.3.1001, Tailwind CSS 4.3.3, and
esbuild v0.28.2. It regenerates templates and assets before compiling so the
embedded files match the tagged source.

## Archives

Each target is a separate archive:

| Target | Archive |
| --- | --- |
| Linux amd64 | `gofer-<version>-linux-amd64.tar.gz` |
| Linux arm64 | `gofer-<version>-linux-arm64.tar.gz` |
| macOS amd64 | `gofer-<version>-darwin-amd64.tar.gz` |
| macOS arm64 | `gofer-<version>-darwin-arm64.tar.gz` |
| Windows amd64 | `gofer-<version>-windows-amd64.zip` |
| Windows arm64 | `gofer-<version>-windows-arm64.zip` |

Every archive contains the binary (`gofer` or `gofer.exe`), `README.md`,
`LICENSE`, and `THIRD_PARTY_NOTICES.txt`. A `checksums.txt` file with SHA-256
hashes is published next to the archives.

The binaries are built with `CGO_ENABLED=0`, `-tags embedded_assets`,
`-trimpath`, and the version stamped into
`internal/buildinfo.Version`.

## Local build

```sh
VERSION=v0.21.0 task release:all
```

That task regenerates templates and assets, checks the attribution files, and
calls [scripts/release-package.sh](../scripts/release-package.sh). To package
already-generated assets without regenerating them:

```sh
VERSION=v0.21.0 bash scripts/release-package.sh
```

`DIST` overrides the output directory (default `dist`).

## Limits

The workflow does not sign Windows binaries, notarize macOS binaries, or build
installers. The Docker image is published separately by the **Docker image**
workflow.
