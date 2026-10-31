# Release and rollback runbook

This runbook covers binary, Docker, and Helm releases, plus the rollback path when a release fails after startup. Run the preflight from the same host and as the same user that runs Gofer.

## Preflight

1. Read the release notes and confirm the target version supports the current data-directory layout.
2. Stop writes and take a verified backup:

   ```sh
   gofer storage backup --dir /var/lib/gofer/data --out /var/backups/gofer/pre-upgrade.tar.gz
   gofer storage verify --from /var/backups/gofer/pre-upgrade.tar.gz
   ```

3. Record the current version and database schema version:

   ```sh
   go version -m /usr/local/bin/gofer
   gofer storage inspect --db /var/lib/gofer/data/gofer.db
   ```

   For an installed release without Go, read the version under **Settings → About**.

4. Keep the previous binary or image digest until the new version has served traffic for at least one full sync cycle.
5. Confirm `/healthz` and `/readyz` are healthy before replacing anything.

## Binary release

Artifacts are built by `scripts/release-build.sh` for Linux and macOS (amd64 and arm64) and Windows (amd64), with `checksums.txt`. Verify before installing:

```sh
sha256sum -c checksums.txt
tar -xzf gofer-vX.Y.Z-linux-amd64.tar.gz
```

Replace the binary atomically:

```sh
install -m 0755 gofer-vX.Y.Z-linux-amd64/gofer /usr/local/bin/gofer.new
mv /usr/local/bin/gofer /usr/local/bin/gofer.previous
mv /usr/local/bin/gofer.new /usr/local/bin/gofer
systemctl restart gofer
```

Windows PowerShell:

```powershell
Get-FileHash .\gofer-vX.Y.Z-windows-amd64.zip -Algorithm SHA256
Expand-Archive .\gofer-vX.Y.Z-windows-amd64.zip -DestinationPath .\gofer-new
Stop-Service Gofer
Copy-Item C:\Gofer\gofer.exe C:\Gofer\gofer.previous.exe
Copy-Item .\gofer-new\gofer.exe C:\Gofer\gofer.exe
Start-Service Gofer
```

Check startup logs for migration errors, then load the sign-in page, one mailbox, one attachment, and `/healthz`. Do not delete the previous binary or the pre-upgrade backup until the release is accepted.

## Docker release

Images are published to `ghcr.io/cristianadrielbraun/gofer` for amd64 and arm64. A release tag publishes the exact version, the minor version, and `latest`; pre-releases publish only their exact version.

```sh
docker pull ghcr.io/cristianadrielbraun/gofer:X.Y.Z
docker compose stop gofer
docker compose run --rm gofer storage backup --dir /data --out /data/backups/pre-upgrade.tar.gz
docker compose run --rm gofer storage verify --from /data/backups/pre-upgrade.tar.gz
docker compose up -d gofer
```

Pin the image tag in `compose.yaml`; do not rely on `latest` for production. The data volume is the only state, so the backup must be copied off that volume before the upgrade.

## Helm release

```sh
helm repo update
helm upgrade gofer oci://ghcr.io/cristianadrielbraun/charts/gofer \
  --version <chart-version> \
  --reuse-values
```

Before upgrading, run a backup from a one-off pod or a maintenance job against the same PVC. The chart uses one replica and `ReadWriteOnce`, so scale the StatefulSet to zero before mounting the volume elsewhere.

```sh
kubectl scale statefulset/gofer --replicas=0
kubectl run gofer-backup --rm -it --restart=Never \
  --image ghcr.io/cristianadrielbraun/gofer:<previous-version> \
  --overrides='{"spec":{"containers":[{"name":"gofer-backup","image":"ghcr.io/cristianadrielbraun/gofer:<previous-version>","command":["gofer","storage","backup","--dir","/data","--out","/data/backups/pre-upgrade.tar.gz"],"volumeMounts":[{"name":"data","mountPath":"/data"}]}],"volumes":[{"name":"data","persistentVolumeClaim":{"claimName":"gofer"}}]}}'
kubectl scale statefulset/gofer --replicas=1
```

After the upgrade, check the pod events, `/healthz`, `/readyz`, and application logs. A failed readiness probe should trigger a rollback before users depend on the new version.

## Rollback decision

Roll back when any of these is true:

- The new version cannot start, or repeatedly fails readiness.
- Sign-in, mailbox sync, attachment download, or a required provider is broken.
- The database migration fails or leaves the service read-only.
- A security regression is confirmed.

Do not roll back by downgrading the binary alone after a forward schema migration. Start with the previous binary against the current database only if the release notes explicitly allow it. Otherwise restore the pre-upgrade backup together with the previous binary.

## Rollback procedure

### Binary

1. Stop Gofer.
2. Preserve the current data directory and logs for analysis.
3. Stop the service and restore the pre-upgrade backup:

   ```sh
   gofer storage restore \
     --from /var/backups/gofer/pre-upgrade.tar.gz \
     --dir /var/lib/gofer/data \
     --force
   ```

4. Replace the binary with the previous version.
5. Start Gofer and check `/healthz`, `/readyz`, sign-in, sync, and an attachment.
6. Confirm the schema version matches the previous release and record the rollback time.

### Docker

```sh
docker compose stop gofer
docker compose run --rm gofer \
  storage restore --from /data/backups/pre-upgrade.tar.gz --dir /data --force
# Set the previous image tag in compose.yaml, then:
docker compose up -d gofer
```

If the image cannot be pulled, use the locally cached digest. Do not delete the failed image or volume until the incident review is complete.

### Helm

```sh
kubectl scale statefulset/gofer --replicas=0
# Restore the PVC from the pre-upgrade backup with a one-off pod, using the
# same restore command as the binary procedure with --dir /data --force.
helm rollback gofer <previous-revision>
kubectl scale statefulset/gofer --replicas=1
kubectl rollout status statefulset/gofer
```

Helm rollback restores Kubernetes objects, not application data. A database that has already migrated forward still needs the pre-upgrade restore.

## Rollback test procedure

Test rollback before a production release, on a copy of the data directory:

1. Start version N and create a mailbox, contact, Calendar event, attachment, and draft.
2. Back up with `gofer storage backup`, verify the archive, and record the schema version.
3. Start version N+1 against the copy; confirm it migrates and serves requests.
4. Stop version N+1, restore the version N backup, and start version N.
5. Confirm the seeded objects are present, `/healthz` and `/readyz` pass, and logs contain no schema or decryption errors.
6. Record the elapsed restore time. The target RTO is one hour.
7. Repeat the test in Docker and Helm at least once per release cycle.

An untested rollback is not a rollback. Store the test date and the archive name with the release record.

## Post-release checks

- [ ] `gofer -version` reports the expected version.
- [ ] `/healthz` and `/readyz` return 200.
- [ ] Sign-in works in every enabled authentication mode.
- [ ] At least one mailbox syncs new mail.
- [ ] An attachment downloads through its signed link.
- [ ] Contacts and Calendar sync for one account.
- [ ] AI analysis and draft generation either work or fail closed with a clear error.
- [ ] Webhook delivery succeeds or is visibly queued.
- [ ] No new migration, decryption, or permission errors in the logs.
- [ ] The previous version, pre-upgrade backup, and checksums are retained.
