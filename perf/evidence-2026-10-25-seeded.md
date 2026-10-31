# Seeded local S2 run — 2026-10-25

Status: S2-shaped local run on a synthetic 10,000-message fixture. Not a
production-sized or three-OS run.

## Environment

- Windows, local loopback, 12 logical processors.
- Go 1.27.3, k6 v2.3.0.
- Gofer open mode, disk assets, `GOFER_AVATAR_PROVIDERS=none`.
- Synthetic database at schema 142, seeded with [seed_load.py](./seed_load.py).

## Fixture

| Data | Count |
| --- | --- |
| Messages | 10,000 (6,000 Inbox, 3,000 Archive, 1,000 Sent) |
| Conversations | 3,334 |
| Contacts | 1,000 plus observed profiles from mail |
| Calendar events | 500 |
| SQLite size before run | 26 MB |

The synthetic account has IMAP sync disabled, so the run measures browsing and
SQLite reads rather than provider reconnects. Calendar sync logs a harmless
missing-URL error for the fixture source.

## Command

```powershell
$env:BASE_URL = 'http://127.0.0.1:18108'
$env:GOFER_COOKIE = 'smoke=1'
$env:APP_PATHS = '/,/mail/folder/bench-inbox/items,/contacts/items,/calendar'
$env:DURATION = '2m'
$env:HEALTH_RPS = '10'
k6 run --summary-export="$env:TEMP\k6-seeded-summary.json" perf\load.js
```

The app scenario used its default S2 ramp: 20 VUs at peak for six minutes. The
health scenario ran for two minutes at 10 requests per second.

## Results

| Metric | Target | Result |
| --- | --- | --- |
| App peak VUs | 20 | 20 |
| Requests | — | 3,286 |
| Failed requests | below 1% | 0 |
| Health p95 | below 250 ms | 2.05 ms |
| App p95 | below 1.5 s | 26.1 ms |
| App median | — | 14.2 ms |
| App slowest | — | 53.9 ms |
| Response data | — | 961 MB |
| k6 exit | 0 | 0 |

Resource samples were taken every five seconds for 396 seconds:

| Metric | Result |
| --- | --- |
| CPU | 20.1 CPU-seconds total, about 5% of one logical core |
| Working set | 56.6 MB start, 64.0 MB peak |
| Private bytes | 80.5 MB peak |
| Handles | 265 peak |

After the run, `/healthz` and `/readyz` returned 200, `PRAGMA integrity_check`
returned `ok`, the database was 35 MB, and the WAL was 37 MB. No panic or error
appeared in the server log beyond the fixture's expected calendar-sync warning.

## Interpretation

The fixture exercises folder pagination, contact listing, calendar rendering,
and the application shell. It does not exercise writes, provider sync, message
send, AI work, or a multi-tenant installation. It is stronger evidence than the
empty-data smoke but still not the Gate G8 S2/S3 result.
