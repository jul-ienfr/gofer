# Seeded local stress run — 2026-10-25

Status: 100-VU stress-shaped run on the synthetic 10,000-message fixture. It
did not reach saturation, so it is a headroom check rather than a measured
breaking point.

## Environment and fixture

Same host, Go 1.27.3, k6 v2.3.0, and [seed_load.py](./seed_load.py) fixture as
[the S2 run](./evidence-2026-10-25-seeded.md): 10,000 messages, 1,000 contacts,
500 events, IMAP sync disabled, avatar lookups disabled.

## Command

```powershell
$env:BASE_URL = 'http://127.0.0.1:18108'
$env:GOFER_COOKIE = 'smoke=1'
$env:APP_PATHS = '/,/mail/folder/bench-inbox/items,/contacts/items,/calendar'
$env:DURATION = '1m'
$env:HEALTH_RPS = '10'
$env:STAGES = '1m:20,2m:60,2m:100,1m:0'
k6 run --summary-export="$env:TEMP\k6-stress-summary.json" perf\load.js
```

The app rose from 20 to 60 to 100 virtual users and held 100 for two minutes.
The health scenario ran for one minute at 10 requests per second.

## Results

| Metric | Target | Result |
| --- | --- | --- |
| Peak app VUs | 100 | 100 |
| Requests | — | 9,565 |
| Request rate | — | 26.5/s |
| Failed requests | below 1% | 0 |
| App p95 | below 1.5 s | 27.4 ms |
| App median | — | 14.4 ms |
| App slowest | — | 83.7 ms |
| Health p95 | below 250 ms | 1.99 ms |
| Response data | — | 4.1 GB |
| k6 exit | 0 | 0 |

Resource samples over 396 seconds:

| Metric | Result |
| --- | --- |
| CPU | 89 CPU-seconds total, about 22% of one logical core |
| Working set | 64.1 MB peak |
| Private bytes | 85.7 MB peak |
| Handles | 346 peak |

After the run, `/healthz` and `/readyz` returned 200, `PRAGMA integrity_check`
returned `ok`, and the database plus WAL sizes were unchanged. No panic or
unexpected error appeared in the log.

## Interpretation

The server kept p95 latency two orders of magnitude below the S3 threshold at
100 VUs. This shows headroom on this host and fixture; it does not identify the
maximum sustainable load, because the run stopped at 100 VUs before errors or
latency crossed the threshold. Writes, provider sync, message send, AI work, and
multi-tenant isolation were not exercised.
