# Local crash and corruption drill — 2026-10-25

Status: process-kill and corrupt-file drill on a seeded synthetic database. It
does not cover Jev, LLM, webhook, provider, or SQLite write-contention chaos.

## Fixture

- Windows, Go 1.27.3, disk-assets build.
- [seed_load.py](./seed_load.py) database: 10,000 messages, 1,000 contacts,
  500 events, 35 MB database.
- Open mode, avatar lookups disabled, IMAP sync disabled by the fixture.

## Crash and restart

1. Started Gofer on port 18109.
2. Requested `/mail/folder/bench-inbox/items`; it returned 200 with 50 message
   rows.
3. Killed the process with `Stop-Process -Force` while it was serving.
4. Restarted on port 18110 against the same database.

Result:

| Check | Result |
| --- | --- |
| `/healthz` after restart | 200 `ok` |
| `/readyz` after restart | 200 `ok` |
| Inbox items after restart | 200, 50 rows |
| `PRAGMA integrity_check` | `ok` |
| Startup recovery errors | none |
| Process count after kill | 0 |

The restart acquired the exclusive database lock normally; the killed process
did not leave a stuck lock or an unreadable WAL.

## Corrupted database

1. Copied the seeded database.
2. Overwrote the first 24 bytes with `not a sqlite database`.
3. Started Gofer on port 18111 against the copy.

Result:

```text
2026/10/31 20:22:57 boot: database path resolved to ...\corrupt.db
2026/10/31 20:22:57 boot: exclusive database lock acquired
2026/10/31 20:22:57 incompatible storage layout: file is not a database (26)
exit=1
```

The server exited non-zero with a clear diagnostic. It did not delete, rewrite,
or start on the corrupt file.

## Gaps

- No kill during an in-flight SQLite write transaction beyond normal page
  serving.
- No forced disk-full or I/O-error test.
- No live Jev, LLM, webhook, or provider failure injection; unit-level fault
  coverage is listed in [phase8-hardening.md](../docs/phase8-hardening.md).
- No production-sized database or multi-tenant instance.
