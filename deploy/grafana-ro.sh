#!/usr/bin/env bash
# Создаёт/обновляет read-only роль grafana_ro для SQL-датасорсов Grafana
# (точная выручка из payment_db, продуктовая витрина из analytics_db).
# Идемпотентен — безопасно запускать повторно.
#
# Запуск из корня репозитория (пароль берётся из deploy/.env; не используйте
# `source deploy/.env` — многострочные JWT-ключи ломают шелл):
#   GRAFANA_RO_PASSWORD=$(grep '^GRAFANA_RO_PASSWORD=' deploy/.env | cut -d= -f2-) bash deploy/grafana-ro.sh
set -euo pipefail

CONTAINER=${PG_CONTAINER:-banya-postgres}
PGUSER=${PG_USER:-banya}
: "${GRAFANA_RO_PASSWORD:?Set GRAFANA_RO_PASSWORD (deploy/.env)}"

# Роль кластерная — создаём один раз; пароль обновляем на каждом запуске.
docker exec -i "$CONTAINER" psql -U "$PGUSER" -d postgres \
  -v ON_ERROR_STOP=1 -v pw="$GRAFANA_RO_PASSWORD" <<'SQL'
SELECT format('CREATE ROLE grafana_ro LOGIN PASSWORD %L', :'pw')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'grafana_ro')
\gexec
SELECT format('ALTER ROLE grafana_ro LOGIN PASSWORD %L', :'pw')
\gexec
ALTER ROLE grafana_ro SET statement_timeout = '5s';
SQL

for db in payment_db analytics_db; do
  echo "Granting read-only on $db..."
  docker exec -i "$CONTAINER" psql -U "$PGUSER" -d "$db" -v ON_ERROR_STOP=1 <<SQL
GRANT CONNECT ON DATABASE $db TO grafana_ro;
GRANT USAGE ON SCHEMA public TO grafana_ro;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO grafana_ro;
-- Будущие таблицы (новые миграции от $PGUSER) тоже читаемы.
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO grafana_ro;
SQL
done

echo "grafana_ro is ready."
