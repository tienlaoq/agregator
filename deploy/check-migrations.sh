#!/usr/bin/env bash
#
# check-migrations.sh — apply every service's *.up.sql FROM SCRATCH under
# ON_ERROR_STOP into throwaway databases and fail if any migration errors.
#
# Why this exists:
#   deploy/migrate.sh pipes each migration through `psql` WITHOUT
#   `-v ON_ERROR_STOP=1`, so a failing statement is logged and skipped while
#   psql still exits 0. A migration that cannot apply on a clean database
#   therefore silently leaves production schema incomplete (missing index,
#   un-converted column type, absent trigger/constraint) with no signal.
#   This guard reproduces a from-scratch apply with strict error handling so CI
#   catches that class of bug on the PR that introduces it.
#
# Scope: this checks the FROM-SCRATCH apply only — it does NOT re-run migrations
# to assert idempotency (the production runner tolerates non-idempotent re-runs
# by design, see migrate.sh). Its job is to ensure a fresh deploy / disaster
# recovery would build a complete schema.
#
# Usage (local):
#   docker run -d --name pg -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgis/postgis:16-3.4
#   PGHOST=localhost PGUSER=postgres PGPASSWORD=postgres bash deploy/check-migrations.sh
#
# Connection is configured via the standard libpq env vars (PGHOST, PGPORT,
# PGUSER, PGPASSWORD). PostGIS image is required because venue-service uses it.

set -uo pipefail

PGHOST="${PGHOST:-localhost}"
PGPORT="${PGPORT:-5432}"
PGUSER="${PGUSER:-postgres}"
export PGHOST PGPORT PGUSER
export PGPASSWORD="${PGPASSWORD:-postgres}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Services in apply order, each mapped to the database it migrates. crm-service
# shares venue_db and FKs into venue-service's schema, so it MUST follow
# venue-service into the same database (mirrors deploy/migrate.sh).
SERVICES=(
  "auth-service:auth_db"
  "user-service:user_db"
  "venue-service:venue_db"
  "crm-service:venue_db"
  "booking-service:booking_db"
  "review-service:review_db"
  "payment-service:payment_db"
  "master-service:master_db"
  "chat-service:chat_db"
  "notification-service:notification_db"
  "api-gateway:support_db"
  "analytics-service:analytics_db"
)

# KNOWN_BROKEN quarantines migrations that fail a from-scratch apply and have NOT
# been fixed yet. Format: "service/filename". The guard still REPORTS them (so
# they stay visible) but does not fail on them. Remove an entry once the
# migration is fixed — the guard flags a quarantined migration that has started
# passing so the stale entry can be deleted.
# Empty: every service's migration chain applies cleanly from scratch. Add a
# "service/filename" entry (with a reason) only to quarantine a newly-discovered
# from-scratch failure that can't be fixed immediately — the guard keeps
# reporting it but won't fail CI on it. (booking-service/006 was the last entry;
# it is now fixed via an IMMUTABLE conversion helper.)
KNOWN_BROKEN=()

is_known_broken() {
  local key="$1"
  # ${arr[@]+...} guard keeps this safe under `set -u` when KNOWN_BROKEN is empty.
  for kb in ${KNOWN_BROKEN[@]+"${KNOWN_BROKEN[@]}"}; do
    [[ "$kb" == "$key" ]] && return 0
  done
  return 1
}

psql_q() { psql -v ON_ERROR_STOP=1 -X -q "$@"; }

unexpected_failures=()
quarantined_seen=()
quarantined_unexpected_pass=()

for pair in "${SERVICES[@]}"; do
  svc="${pair%%:*}"
  db="${pair##*:}"
  mig_dir="$ROOT/services/$svc/migrations"

  [[ -d "$mig_dir" ]] || { echo "skip $svc (no migrations dir)"; continue; }

  # Fresh database per (service-group). crm reuses venue_db, so only create when
  # absent.
  if ! psql_q -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='$db'" | grep -q 1; then
    psql_q -d postgres -c "CREATE DATABASE \"$db\"" >/dev/null
  fi

  shopt -s nullglob
  for f in "$mig_dir"/*.up.sql; do
    key="$svc/$(basename "$f")"
    if out="$(psql_q -d "$db" -f "$f" 2>&1)"; then
      if is_known_broken "$key"; then
        # A quarantined migration unexpectedly applied cleanly — flag for removal.
        quarantined_unexpected_pass+=("$key")
        echo "✅ $key (was quarantined — now applies cleanly, remove from KNOWN_BROKEN)"
      else
        echo "✅ $key"
      fi
    else
      err="$(printf '%s\n' "$out" | grep -m1 'ERROR:' || printf '%s' "$out" | tail -1)"
      if is_known_broken "$key"; then
        quarantined_seen+=("$key")
        echo "⚠️  $key — KNOWN BROKEN: $err"
      else
        unexpected_failures+=("$key")
        echo "❌ $key — $err"
      fi
    fi
  done
  shopt -u nullglob
done

echo
echo "──────── migration from-scratch check ────────"
echo "quarantined (known-broken, not failing CI): ${#quarantined_seen[@]}"
echo "unexpected failures:                        ${#unexpected_failures[@]}"

if [[ "${#quarantined_unexpected_pass[@]}" -gt 0 ]]; then
  echo
  echo "The following quarantined migrations now apply cleanly — remove them from"
  echo "KNOWN_BROKEN in deploy/check-migrations.sh:"
  printf '  - %s\n' "${quarantined_unexpected_pass[@]}"
fi

if [[ "${#unexpected_failures[@]}" -gt 0 ]]; then
  echo
  echo "These migrations fail a from-scratch apply. deploy/migrate.sh would skip"
  echo "them silently, leaving the production schema incomplete. Fix them (or, if"
  echo "intentionally deferred, add to KNOWN_BROKEN with a reason):"
  printf '  - %s\n' "${unexpected_failures[@]}"
  exit 1
fi

echo "OK — all migrations apply from scratch (quarantined entries excluded)."
