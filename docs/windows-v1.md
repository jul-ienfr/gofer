# Windows v1 support

Gofer runs natively on Windows 10/11 and Windows Server 2022. This page covers
the fork-specific changes and the local build path.

## Build

Requirements:

- Go 1.27.3
- Node.js 22+
- `templ` v0.3.1001
- `@tailwindcss/cli` v4.3.3

```powershell
go install github.com/a-h/templ/cmd/templ@v0.3.1001
npm install --no-save @tailwindcss/cli@4.3.3
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1
```

The script generates templ views, compiles `assets/css/output.css`, minifies
`assets/js`, and writes `dist/gofer.exe` with embedded assets.

To build the complete portable set (Windows, Linux and macOS, x86-64 and ARM64)
with checksums:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-release.ps1 -Version v1.0.0
```

Artifacts and `SHA256SUMS` are written to `dist/`.

If the execution policy blocks local scripts, run the script through
`powershell.exe -ExecutionPolicy Bypass -File` or sign it.

## Tests

```powershell
go test ./internal/storage ./internal/authoperator ./internal/store ./internal/handler -count=1
go vet ./...
```

CI runs the same commands and a smoke test of `/healthz`, `/`, and the compiled
CSS in `.github/workflows/windows.yml`.

## SQLite paths

SQLite file URIs are built with `storage.SQLiteFileURL`. On Windows, a path such
as `C:\mail\gofer.db` becomes `file:///C:/mail/gofer.db`; the old form
`file:C:/mail/gofer.db` made SQLite treat `C:` as a URI authority and fail with
`invalid uri authority`.

## Auth control endpoint

Unix builds use a Unix socket at `GOFER_DB_PATH.control/socket`. Windows has no
compatible Unix socket, so the same path holds a JSON endpoint:

```json
{"network":"tcp","address":"127.0.0.1:61304","token":"..."}
```

The server listens on loopback only and requires
`Authorization: Bearer <token>` for every mutating `gofer auth` command. The
endpoint file is created with mode `0600`; the token is random per server start.
A stale endpoint is detected as a refused connection and the command falls back
to running locally, exactly like a stale Unix socket.

## AI assistant

The AI features are disabled until both a Jev key and, for drafting, an
OpenAI-compatible endpoint are configured:

```powershell
$env:GOFER_JEV_API_KEY = "..."
$env:GOFER_LLM_BASE_URL = "http://localhost:11434/v1"
$env:GOFER_LLM_MODEL = "qwen2.5:7b"
gofer.exe
```

`GOFER_JEV_URL` and `GOFER_LLM_BASE_URL` must use HTTPS outside localhost. Then
open **Settings → AI assistant** and enable analysis and/or drafting. Analysis
runs only on arrived messages outside Drafts, Sent and Trash. Drafting opens a
prefilled composer draft for human review; it never sends mail. The same tab
manages encrypted routing webhooks; a signed delivery is sent only when the
project allows external routing and the route has a `webhook` action. Optional
auto-spam and RFC 8058 one-click unsubscribe stay off until enabled globally and
on the project; Junk moves can be undone for seven days from the same panel.

## Windows test hardening

Two upstream tests depended on the host environment and are now stable on
Windows:

- `internal/mail/imap` ignores `X-EsetId`/`Old-X-EsetId` headers injected by a
  local antivirus when comparing the fetched message.
- `internal/notifications` accepts the intentional 303 redirect for a foreign
  or missing folder, while still asserting that no foreign data is returned.

Symlink tests are skipped on Windows because creating symlinks requires
Developer Mode or `SeCreateSymbolicLinkPrivilege`. POSIX permission assertions
are skipped for the same reason; file contents are still checked.
