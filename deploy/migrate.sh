#!/usr/bin/env bash
set -e

CONTAINER=${PG_CONTAINER:-banya-postgres}
PGUSER=${PG_USER:-banya}

apply_migrations() {
  local svc=$1
  local db=$2
  for f in services/$svc/migrations/*.up.sql; do
    if [ -f "$f" ]; then
      echo "Applying $f to $db..."
      docker exec -i "$CONTAINER" psql -U "$PGUSER" -d "$db" < "$f"
    fi
  done
}

apply_migrations auth-service auth_db
apply_migrations user-service user_db
apply_migrations venue-service venue_db
# crm-service shares venue_db in phase B (strangler-fig); its migrations are
# CREATE TABLE IF NOT EXISTS so re-running on a DB that already has venue_staff
# / venue_crm_tasks (from the old venue-service migrations 012/013) is a no-op.
apply_migrations crm-service venue_db
apply_migrations booking-service booking_db
apply_migrations review-service review_db
apply_migrations payment-service payment_db
apply_migrations master-service master_db
apply_migrations chat-service chat_db
apply_migrations api-gateway support_db

echo "All migrations applied."
