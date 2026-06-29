# Database migrations in Kubernetes

In production the schema is applied by a **Helm pre-install/pre-upgrade hook Job**
(`templates/migrate-job.yaml`), not by `deploy/migrate.sh`. The shell script only
works against the docker-compose Postgres via `docker exec` and is dev-only.

The hook runs the `migrator` image (`services/migrator`), which bundles every
service's SQL under `/migrations/<service>/` and applies it with the shared,
versioned, idempotent migrator (`pkg/dbmigrate`). It records applied files in a
`schema_migrations` table per database, so re-runs are no-ops. The hook has
weight `-5`, so it completes **before** the service Deployments roll out — new
code never starts against a stale schema.

`venue_db` is shared (strangler-fig phase B): the migrator applies
`venue-service` migrations first, then `crm-service` migrations, because CRM
tables foreign-key `venues(id)`.

## Prerequisites (Yandex Managed PostgreSQL)

The migrator applies schema only — it does **not** create databases (the app
role has no `CREATEDB` on managed Postgres). Before the first deploy:

### 1. Create the databases

Owned by the application role (`banya` by default), via the YC console/API:

```
auth_db  user_db  venue_db  booking_db  review_db  payment_db
master_db  chat_db  notification_db  support_db  analytics_db
```

### 2. Enable required extensions

Migrations run `CREATE EXTENSION IF NOT EXISTS`, but managed Postgres requires
the extension to be enabled on the cluster first (console → Extensions, or API):

| Database     | Extensions            |
|--------------|-----------------------|
| `venue_db`   | `postgis`, `btree_gist` |
| `booking_db` | `btree_gist`          |

(`pg_trgm` is **not** required — no migration depends on it.)

### 3. Create the `migrator-env` secret

The Job reads standard `PG_*` connection params (the database name is chosen
per entry by the migrator, so do **not** set `PG_DB`):

```bash
kubectl -n agregator create secret generic migrator-env \
  --from-literal=PG_HOST=<cluster-host>.mdb.yandexcloud.net \
  --from-literal=PG_PORT=6432 \
  --from-literal=PG_USER=banya \
  --from-literal=PG_PASSWORD=<password> \
  --from-literal=PG_SSLMODE=verify-full
```

For `verify-full`, mount the YC CA bundle and point pgx at it, or use `require`
if CA distribution is handled separately. The role must have access to every
database listed above.

## Running

Migrations run automatically on `helm upgrade --install` (the CD pipeline). To
run them standalone without deploying services:

```bash
helm upgrade --install agregator deploy/helm/agregator \
  --namespace agregator --set migrator.enabled=true \
  --set services.*.enabled=false   # optional: hook still runs
```

A failed migration fails the release (`--atomic` rolls back). Inspect logs:

```bash
kubectl -n agregator logs job/agregator-migrate
```
