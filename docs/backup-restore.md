# Backup and restore runbook

Back up the whole Gofer data directory, not only the database. Mail attachments, encryption keys, provider subscriptions, and per-user databases all live under the same directory.

## What the backup contains

`gofer storage backup` writes a `.tar.gz` archive with:

- A consistent SQLite snapshot of every SQLite database in the data directory, taken with `VACUUM INTO`.
- Attachments and other regular files, unchanged.
- `manifest.json` with the format version, schema version, file list, sizes, modes, and SHA-256 checksums.

The command skips:

- `backups/` in the data directory, so archives do not contain earlier archives.
- SQLite `-wal`, `-shm`, and journal files.
- The output archive itself.
- Sockets and other non-regular files.

The archive contains `secret.key` when it lives inside the data directory. That key decrypts stored mailbox credentials. Store backups with the same care as the running server, encrypt them at rest, and restrict who can read them.

## Recovery targets

| Target | Value | How to meet it |
| --- | --- | --- |
| Recovery point objective (RPO) | 15 minutes | Run a backup at least every 15 minutes, or take a storage snapshot at that interval. |
| Recovery time objective (RTO) | 1 hour | Keep the latest verified archive on the recovery host, test the restore quarterly, and keep one previous binary version. |

A backup is not a recovery point until it has been verified. Run `gofer storage verify --from <archive>` after every scheduled backup. The verification reads the whole archive, checks every checksum, and rejects unsafe paths, duplicate or unlisted entries, and unsupported entry types. Periodically restore to a scratch directory as an end-to-end check.

### Recorded local drill — 2026-10-25

On one Windows host, a 35.7 MB seeded database (10,000 messages, 1,000 contacts, 500 events) produced a 4.27 MB archive. Backup took 403 ms, verification passed, and restore to a new directory took 120 ms. The restored server returned 200 from `/healthz` and `/readyz`, listed 50 Inbox rows, passed `PRAGMA integrity_check`, and retained all 10,000 messages, 1,000 contacts, and 500 events.

This measures local archive and restore speed only. It does not demonstrate the 15-minute RPO or one-hour RTO on production storage, and it does not include attachments or a live write load.

## Create a backup

Stop Gofer for a fully quiescent backup. If the service must stay online, the SQLite snapshot remains consistent, but files written during the copy may be captured at different moments.

```sh
gofer storage backup \
  --dir /var/lib/gofer/data \
  --db /var/lib/gofer/data/gofer.db \
  --out /var/backups/gofer/gofer-$(date -u +%Y%m%dT%H%M%SZ).tar.gz
```

`--dir` defaults to the directory of `GOFER_DB_PATH`, `--db` defaults to `<dir>/gofer.db`, and `--out` defaults to `<dir>/backups/gofer-<timestamp>.tar.gz`. Use `--force` only when replacing a known archive.

Windows PowerShell:

```powershell
$stamp = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ')
gofer storage backup --dir C:\Gofer\data --out "D:\Backups\gofer-$stamp.tar.gz"
```

Docker:

```sh
docker compose exec gofer \
  gofer storage backup --dir /data --out /data/backups/manual.tar.gz
```

Then copy the archive off the volume. A backup left only on the same disk does not survive disk loss.

## Verify a backup

Run the archive check first:

```sh
gofer storage verify --from /var/backups/gofer/gofer-20260101T000000Z.tar.gz
```

Then restore it to a scratch directory and compare the printed schema version and file count with the source installation. Do not point this check at the live data directory.

```sh
gofer storage restore \
  --from /var/backups/gofer/gofer-20260101T000000Z.tar.gz \
  --dir /var/tmp/gofer-restore-check

gofer storage inspect --db /var/tmp/gofer-restore-check/gofer.db
```

The restore command rejects path traversal, duplicate entries, symlinks and other unsupported archive types, missing files, unlisted files, checksum mismatches, a missing manifest, a manifest larger than 10 MiB, more than 1,000,000 entries, and more than 100 GiB of extracted data. It also rejects a database path outside the target data directory.

## Restore over an existing installation

1. Stop Gofer and any process holding the data directory.
2. Take a fresh backup of the current data directory.
3. Restore the chosen archive with `--force`.
4. Start Gofer and check `/healthz`, sign-in, one mailbox sync, and one attachment download.
5. If the schema is newer than the running binary, restore the matching binary version instead of forcing the schema. `--allow-newer-schema` exists only for controlled recovery where the newer schema is understood.

```sh
gofer storage restore \
  --from /var/backups/gofer/gofer-20260101T000000Z.tar.gz \
  --dir /var/lib/gofer/data \
  --force
```

The restore creates `backups/pre-restore-<timestamp>.tar.gz` inside the data directory before changing data. Keep that archive until the restored installation has been checked. `--no-safety-backup` skips it and is intended only for low-disk emergencies.

`--force` overwrites files named in the archive. It does not delete unrelated files already in the data directory. For a disaster recovery where the old data may be corrupt, restore into an empty directory and switch the service to it.

## Docker and Kubernetes

Docker Compose:

```sh
docker compose stop gofer
docker compose run --rm gofer \
  storage restore --from /data/backups/gofer-20260101T000000Z.tar.gz --dir /data --force
docker compose start gofer
```

Helm, with the release's data volume mounted by a one-off pod:

```sh
kubectl scale statefulset/gofer --replicas=0
kubectl run gofer-restore --rm -it --restart=Never \
  --image ghcr.io/cristianadrielbraun/gofer:<version> \
  --overrides='{"spec":{"containers":[{"name":"gofer-restore","image":"ghcr.io/cristianadrielbraun/gofer:<version>","command":["gofer","storage","restore","--from","/data/backups/archive.tar.gz","--dir","/data","--force"],"volumeMounts":[{"name":"data","mountPath":"/data"}]}],"volumes":[{"name":"data","persistentVolumeClaim":{"claimName":"gofer"}}]}}'
kubectl scale statefulset/gofer --replicas=1
```

Replace the image tag and PVC name with the values from `helm get values`. The chart uses one replica and a `ReadWriteOnce` volume, so scale down before mounting the restore pod.

## Schedule and retention

A practical schedule for a single installation:

- Every 15 minutes: backup to a local staging directory.
- Hourly: copy to a different host or object store.
- Daily: keep 7 daily archives.
- Weekly: keep 4 weekly archives.
- Monthly: keep 3 monthly archives.

Retention is a policy choice; the important controls are off-host copies, encryption, and a restore test at least quarterly. Record the newest verified restore test date with the backup set.

## Incident checklist

- [ ] Confirm the incident and stop writes to the data directory.
- [ ] Record the time of the last known good state.
- [ ] Select the newest archive created before that time.
- [ ] Verify the archive and restore it to a scratch directory.
- [ ] Check schema version, sign-in, mailbox sync, contacts, Calendar, and an attachment.
- [ ] Restore over the live directory with `--force` and keep the safety backup.
- [ ] Start Gofer, check `/healthz` and `/readyz`, and watch logs for migration or decryption errors.
- [ ] Record actual RPO and RTO, the archive used, and any data lost since the last backup.
- [ ] Add a regression test or monitoring change for the cause.

## Failure modes

| Symptom | Action |
| --- | --- |
| `backup ... already exists` | Choose a new `--out` or pass `--force`. |
| `database ... is outside data directory` | Pass a database path under `--dir`. |
| `checksum mismatch` | Treat the archive as corrupt; use an older verified archive. |
| `backup schema version N is newer` | Restore the binary that matches the archive, or use `--allow-newer-schema` only after confirming compatibility. |
| Restore stops with `data directory ... is not empty` | Stop Gofer, confirm the target, then pass `--force`. |
| Decryption errors after restore | Restore `secret.key` with the databases, or set the original `GOFER_SECRET_KEY`. |
| Disk full during restore | Free space and retry; the command extracts to a temporary directory before copying files into place. |
