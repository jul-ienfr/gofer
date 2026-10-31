# Load and chaos testing

These assets cover the Phase 8 load and fault-injection requirements. They are designed for a disposable copy of an installation, never production.

## Load test with k6

Install k6 from [k6.io](https://k6.io/docs/get-started/installation/) or a package manager, then start Gofer with a test database and enough seeded data.

### Seed data

Use a copy of a real database or create a test account and seed it with representative data. The acceptance thresholds assume:

- 10,000 messages across at least three mailboxes.
- 1,000 contacts.
- 500 Calendar events.
- At least one attachment over 5 MiB.
- At least one IMAP, one CalDAV, and one CardDAV account, or realistic mocks.

An empty database measures templates and SQLite startup, not the product under normal load.

For a disposable synthetic fixture, initialize the schema once, copy the database, then run [seed_load.py](./seed_load.py):

```sh
python perf/seed_load.py --db /tmp/gofer-load/gofer.db \
  --messages 10000 --contacts 1000 --events 500
```

The script populates the first active user with three mail folders, folder/thread state, search rows, contacts, and 500 calendar events. It disables IMAP sync for the synthetic account so the server does not reconnect while measuring. Set `GOFER_AVATAR_PROVIDERS=none` on the test server to keep avatar lookups from adding external latency.

For a full page mix, include the list endpoints rather than only their shells:

```sh
$env:APP_PATHS = '/,/mail/folder/bench-inbox/items,/contacts/items,/calendar'
```

Replace `bench-inbox` with a real folder ID when seeding a copy of an existing database.

### S2 expected peak

| Metric | Target |
| --- | --- |
| Concurrent browsing users | 20 |
| Health checks | 10 requests/second |
| Page requests | 10–20 virtual users |
| Error rate | below 1% |
| Health p95 | below 250 ms |
| App p95 | below 1.5 s |
| App p99 | below 3 s |

Run:

```sh
BASE_URL=http://127.0.0.1:8090 \
GOFER_COOKIE='gofer_session=<session>' \
k6 run perf/load.js
```

PowerShell:

```powershell
$env:BASE_URL = 'http://127.0.0.1:8090'
$env:GOFER_COOKIE = 'gofer_session=<session>'
k6 run .\perf\load.js
```

Without `GOFER_COOKIE`, only the health scenario runs. This is useful for a liveness check but does not exercise the application.

Useful overrides:

| Variable | Default | Purpose |
| --- | --- | --- |
| `DURATION` | `5m` | Health scenario duration. |
| `HEALTH_RPS` | `10` | Health requests per second. |
| `APP_PATHS` | `/,/calendar,/contacts` | Comma-separated pages for the app scenario. |
| `SKIP_APP` | unset | Set to `1` to run only health checks. |
| `STAGES` | built-in S2 ramp | Comma-separated `duration:target` pairs for the app scenario. |

### S3 stress

Raise the app scenario until errors or latency cross the thresholds, then hold for five minutes. The goal is graceful failure: no panic, no corrupted SQLite file, no stuck worker, and a clean recovery after the load stops.

```sh
BASE_URL=http://127.0.0.1:8090 GOFER_COOKIE='...' \
STAGES='2m:20,5m:50,5m:100,2m:0' k6 run perf/load.js
```

PowerShell:

```powershell
$env:BASE_URL = 'http://127.0.0.1:8090'
$env:GOFER_COOKIE = 'gofer_session=<session>'
$env:STAGES = '2m:20,5m:50,5m:100,2m:0'
k6 run .\perf\load.js
```

This overrides the script stages. Record:

- Maximum sustained request rate before p95 crosses 1.5 s.
- First error class and rate.
- Memory and CPU of the Gofer process.
- SQLite `busy` errors and WAL size.
- Whether `/healthz` and `/readyz` stayed healthy.
- Time to full recovery after the load stops.

### Interpreting results

Use the k6 summary and the Gofer logs together. A 500 caused by a missing test account is a test-fixture problem, not a product result. Keep the k6 summary, the Gofer log window, and the exact commit in the evidence file.

Do not tune thresholds to make a run pass. If the product misses S2, record the miss and fix it.

## Chaos drills

Run these on a disposable installation with a verified backup. Each drill has an expected behavior and a recovery check.

### Jev/LLM provider failure

1. Configure an AI provider endpoint that returns 500, then a timeout, then a valid response.
2. Queue an analysis or draft job.
3. Observe retries, backoff, and job state in the UI or database.
4. Confirm no draft is sent automatically and no partial output is stored as final.

Expected: the job retries with a bounded backoff, the failure is visible, and a later retry succeeds. A malformed response is rejected without a panic.

Automated coverage: `go test ./internal/jev ./internal/llm ./internal/aiworker ./internal/aipipeline -count=1`.

### Webhook failure and forgery

1. Point a webhook at a host that refuses connections, then at one returning 500, then at a working receiver.
2. Confirm the delivery retries and the receiver sees `X-Gofer-Timestamp`, `X-Gofer-Signature`, and an idempotency key.
3. Send a request with a changed body, changed timestamp, and a forged signature.
4. Confirm the receiver rejects it and Gofer does not treat the failure as success.

Expected: network failures are retryable, permanent 4xx errors are not retried forever, and signatures use constant-time comparison.

Automated coverage: `go test ./internal/routing -count=1`.

### Database lock or disk pressure

1. Hold an exclusive SQLite transaction from a separate process for 30 seconds.
2. Perform a mailbox sync, a settings change, and a draft save.
3. Confirm requests fail with a bounded error or wait within the busy timeout; no partial rows are committed.
4. Fill a scratch volume until writes fail. Confirm the error is visible and the existing database still opens.
5. Run `PRAGMA integrity_check` after recovery.

Expected: no corruption, no infinite wait, and a clear error in the log. The service recovers when the lock or disk pressure is removed.

### Process crash during work

1. Start a sync and an AI job.
2. Kill the process with `SIGKILL` (or `Stop-Process -Force` on Windows) during the work.
3. Restart Gofer.
4. Check the jobs, the mailbox, and the SQLite integrity.

Expected: leases expire, jobs are retried or marked failed, and no duplicate mailbox messages or duplicate sends appear.

### Provider outage

1. Block outbound traffic to the IMAP, SMTP, CalDAV, or CardDAV host at the firewall.
2. Trigger a sync and a send.
3. Restore connectivity and retry.

Expected: sync backs off, send reports a clear failure and keeps the draft, and the next attempt succeeds. A send must never be reported as successful when the provider did not accept it.

## Recorded runs

- [2026-10-25 local smoke](./evidence-2026-10-25-local.md) — script and empty-shell validation only; not S2/S3 evidence.
- [2026-10-25 seeded S2](./evidence-2026-10-25-seeded.md) — 10,000-message synthetic fixture; S2-shaped pass on one Windows host.
- [2026-10-25 seeded stress](./evidence-2026-10-25-stress.md) — populated 100-VU stress run; see the evidence file for the result.
- [2026-10-25 chaos](./evidence-2026-10-25-chaos.md) — forced kill/restart and corrupt-database startup checks.

## Evidence template

```text
Date:
Commit:
Environment (OS, Go, k6, CPU/RAM):
Database size / message count:
Scenario: S2 / S3 / chaos
Command:
Thresholds: pass / fail (attach summary)
Observed p95 / p99:
Observed error rate:
Recovery time:
Follow-up issue:
```
