# Local load smoke — 2026-10-25

Status: fixture validation only. This is not S2 or S3 evidence.

## Environment

- Windows, local loopback, no container.
- Go 1.27.1 (before the security update to Go 1.27.3).
- k6 v2.3.0.
- Gofer open mode, empty database at schema version 142 (4 KiB database plus WAL).
- Uncommitted working tree; record the commit before comparing runs.

## Command

```powershell
$env:BASE_URL = 'http://127.0.0.1:18100'
$env:GOFER_COOKIE = 'smoke=1'
$env:DURATION = '2m'
$env:HEALTH_RPS = '10'
k6 run --summary-export="$env:TEMP\k6-summary.json" perf\load.js
```

The dummy cookie enables the app scenario; open mode ignores it. The app scenario ran its default ramp to 20 VUs for six minutes. The health scenario ran for two minutes at 10 requests per second.

## Results

| Metric | Result |
| --- | --- |
| Requests | 3,295 |
| Failed requests | 0 |
| Checks | 8,685 passed, 0 failed |
| Health p95 | 2.09 ms |
| App p95 | 20.07 ms |
| Slowest request | 65.38 ms |
| Response data | 855.6 MB |
| k6 exit | 0 |

## Interpretation

The run validates the k6 script, the health endpoint, the application shell, and the page routes under a light concurrent load. It does not exercise a populated mailbox, SQLite write contention, message rendering at scale, provider sync, AI work, or mail send.

Do not count this as the S2 gate. The next run must use the seeded fixture described in [README.md](./README.md), capture Gofer CPU, memory, and logs, and hold peak load for at least five minutes.
