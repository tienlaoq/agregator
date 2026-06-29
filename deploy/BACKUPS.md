# Backups & restore

Three independent layers protect the platform's state:

| Layer | What | Mechanism | RPO |
|---|---|---|---|
| 1 | PostgreSQL (primary DR) | Yandex Managed PostgreSQL automated backups + PITR | ~minutes (PITR) |
| 2 | PostgreSQL (defense-in-depth) | logical `pg_dump` CronJob → Object Storage | 1 day (configurable) |
| 3 | Media (photos) | Object Storage bucket versioning | continuous |

Layer 1 is the real disaster-recovery path. Layer 2 survives what Layer 1 cannot:
accidental `DROP`, cluster deletion, or loss of the cloud account — and restores
to any Postgres, anywhere. Redis and NATS hold only transient state and are not
backed up.

---

## Layer 1 — Yandex Managed PostgreSQL (PITR)

Configure on the cluster (console / `yc managed-postgresql cluster update`):

- **Backup retention**: 14 days (or per your policy).
- **Backup window**: off-peak, e.g. 01:00–02:00 UTC.
- **PITR**: enabled by default on Managed PG — restore to any second within the
  retention window via *Create cluster from backup* → point in time.

Restore creates a **new** cluster; repoint the app's `PG_HOST` (the per-service
`*-env` secrets and `migrator-env` / `pg-backup-env`) at it after recovery.

## Layer 2 — logical pg_dump CronJob

Built from [deploy/backup](backup) (`pg-backup` image: `postgres:16-alpine` +
`mc`). Deployed by the Helm chart as a CronJob
([templates/backup-cronjob.yaml](helm/agregator/templates/backup-cronjob.yaml)),
schedule and retention in [values.yaml](helm/agregator/values.yaml) under
`backup:` (default: daily 02:00 UTC, keep 14 days).

It dumps every database (`-Fc` custom format) to:

```
s3://<bucket>/<prefix>/YYYY/MM/DD/HHMMSSZ/<db>.dump
                                          /MANIFEST
```

### Required secret `pg-backup-env`

libpq-native names so `pg_dump` reads them directly; **do not set PGDATABASE**
(the script iterates all databases):

```bash
kubectl -n agregator create secret generic pg-backup-env \
  --from-literal=PGHOST=<cluster-host>.mdb.yandexcloud.net \
  --from-literal=PGPORT=6432 \
  --from-literal=PGUSER=banya \
  --from-literal=PGPASSWORD=<password> \
  --from-literal=PGSSLMODE=verify-full \
  --from-literal=S3_ENDPOINT=https://storage.yandexcloud.net \
  --from-literal=S3_ACCESS_KEY=<static-access-key-id> \
  --from-literal=S3_SECRET_KEY=<static-access-key-secret> \
  --from-literal=S3_BUCKET=<backup-bucket>
```

The S3 credentials are a Yandex Cloud **static access key** for a service account
with write access to the backup bucket. Keep this bucket **separate** from the
media bucket, and ideally with **object-lock / versioning** so retention sweeps
can't be abused to wipe history.

### Run on demand

```bash
kubectl -n agregator create job --from=cronjob/agregator-pg-backup backup-manual
kubectl -n agregator logs -f job/backup-manual
```

### Restore a database from a dump

```bash
# 1. fetch the dump (from a pod with mc, or locally with the bucket configured)
mc cp backup/<bucket>/postgres/2026/06/28/020000Z/venue_db.dump ./

# 2. restore into the target database (clean first; --no-owner for managed PG)
PGPASSWORD=<pw> pg_restore \
  --host=<host> --port=6432 --username=banya \
  --dbname=venue_db --no-owner --clean --if-exists \
  venue_db.dump
```

`venue_db` carries both venue-service and crm-service schemas — restoring it
recovers CRM too. After restore, app pods reconnect automatically.

## Layer 3 — media (Object Storage)

Photos live in the S3-compatible media bucket (`MINIO_BUCKET`, default `photos`).
Enable **bucket versioning** + a lifecycle rule (e.g. keep noncurrent versions
30 days) on the media bucket so overwrites/deletes are recoverable. For a second
region/account copy, add an `mc mirror` CronJob (out of scope for this change).

---

## Verify backups regularly

A backup you have never restored is a hypothesis. Quarterly: restore the latest
`pg_dump` set into a throwaway database and run a smoke check, and exercise a YC
PITR restore into a temporary cluster.
