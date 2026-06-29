#!/bin/sh
# Logical PostgreSQL backup: pg_dump every database in custom format and upload
# to an S3-compatible bucket (Yandex Object Storage / MinIO) via the mc client.
# Runs as a Helm CronJob (deploy/helm/agregator/templates/backup-cronjob.yaml).
#
# This is defense-in-depth, independent of Yandex Managed PostgreSQL's automated
# snapshots + PITR: it survives accidental DROP, cluster deletion, and account
# loss, and restores to any Postgres anywhere. See deploy/BACKUPS.md.
#
# Connection uses libpq-native env (so pg_dump picks them up directly):
#   PGHOST PGPORT PGUSER PGPASSWORD PGSSLMODE  [PGSSLROOTCERT]
# Destination:
#   S3_ENDPOINT S3_ACCESS_KEY S3_SECRET_KEY S3_BUCKET
# Tunables:
#   BACKUP_PREFIX  (default "postgres")  key prefix inside the bucket
#   RETENTION_DAYS (default "14")        delete dumps older than this
set -eu

: "${PGHOST:?PGHOST is required}"
: "${PGUSER:?PGUSER is required}"
: "${PGPASSWORD:?PGPASSWORD is required}"
: "${S3_ENDPOINT:?S3_ENDPOINT is required}"
: "${S3_ACCESS_KEY:?S3_ACCESS_KEY is required}"
: "${S3_SECRET_KEY:?S3_SECRET_KEY is required}"
: "${S3_BUCKET:?S3_BUCKET is required}"

PGPORT="${PGPORT:-5432}"
PGSSLMODE="${PGSSLMODE:-require}"
BACKUP_PREFIX="${BACKUP_PREFIX:-postgres}"
RETENTION_DAYS="${RETENTION_DAYS:-14}"
export PGHOST PGPORT PGUSER PGPASSWORD PGSSLMODE

# Keep this list in sync with services/migrator/cmd/main.go (the set of databases
# the platform owns) and deploy/init-databases.sql.
DATABASES="auth_db user_db venue_db booking_db review_db payment_db master_db chat_db notification_db support_db analytics_db"

ts="$(date -u +%Y/%m/%d/%H%M%SZ)"
dest="backup/${S3_BUCKET}/${BACKUP_PREFIX}/${ts}"
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

mc --quiet alias set backup "$S3_ENDPOINT" "$S3_ACCESS_KEY" "$S3_SECRET_KEY"

for db in $DATABASES; do
	echo "dumping ${db} ..."
	# custom format (-Fc): compressed, selective restore via pg_restore.
	pg_dump --dbname="$db" --format=custom --no-owner --no-privileges \
		--file="${workdir}/${db}.dump"
	mc --quiet cp "${workdir}/${db}.dump" "${dest}/${db}.dump"
done

# Manifest records which databases this snapshot contains, for restore tooling.
printf '%s\n' $DATABASES >"${workdir}/MANIFEST"
mc --quiet cp "${workdir}/MANIFEST" "${dest}/MANIFEST"

# Retention: drop snapshots older than RETENTION_DAYS. Best-effort — a failure
# here must not fail the backup that already succeeded above.
mc --quiet rm --recursive --force --older-than "${RETENTION_DAYS}d" \
	"backup/${S3_BUCKET}/${BACKUP_PREFIX}/" || echo "warning: retention sweep failed"

echo "backup complete: ${BACKUP_PREFIX}/${ts}"
