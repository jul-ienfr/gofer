#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ -z "${VERSION:-}" ]]; then
  echo "VERSION is required. Example: VERSION=v0.1.0-alpha.1 task release:all" >&2
  exit 1
fi

task generate
./scripts/licenses.sh --check

VERSION="$VERSION" bash ./scripts/release-package.sh
