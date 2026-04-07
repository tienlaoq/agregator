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
apply_migrations booking-service booking_db
apply_migrations review-service review_db
apply_migrations payment-service payment_db

echo "All migrations applied."
