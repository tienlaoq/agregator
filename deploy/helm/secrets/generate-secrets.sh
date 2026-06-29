#!/usr/bin/env bash
# Generate strong secrets and create the per-service *-env Kubernetes secrets the
# Helm chart consumes (envFromSecrets). Secrets NEVER touch git or deploy/.env.
#
# What it generates (random / openssl): PG_PASSWORD, INTERNAL_SERVICE_TOKEN,
# CURSOR_HMAC_KEY, and the ES256 JWT keypair (JWT_EC_PRIVATE_KEY / _PUBLIC_KEY).
#
# What it reads from the environment (you provide; third-party / cloud values):
#   required: PG_HOST  MINIO_ACCESS_KEY  MINIO_SECRET_KEY
#             S3_ENDPOINT  S3_ACCESS_KEY  S3_SECRET_KEY  S3_BUCKET
#   optional: PG_PORT(6432) PG_USER(banya) PG_SSLMODE(verify-full)
#             VK_CLIENT_ID VK_CLIENT_SECRET SMTP_USER SMTP_PASSWORD
#             TELEGRAM_BOT_TOKEN TELEGRAM_CHAT_ID SUPPORT_HELPDESK_WEBHOOK_TOKEN
#
# Usage:
#   PG_HOST=… MINIO_ACCESS_KEY=… … ./generate-secrets.sh [-n NAMESPACE] [--print]
#     -n, --namespace   target namespace (default: agregator)
#     --print           print manifests to stdout instead of applying (no cluster
#                       needed; safe to inspect — contains plaintext secrets)
set -euo pipefail

NAMESPACE="agregator"
PRINT_ONLY=0
while [ $# -gt 0 ]; do
	case "$1" in
		-n|--namespace) NAMESPACE="$2"; shift 2 ;;
		--print) PRINT_ONLY=1; shift ;;
		-h|--help) sed -n '2,30p' "$0"; exit 0 ;;
		*) echo "unknown arg: $1" >&2; exit 2 ;;
	esac
done

command -v openssl >/dev/null || { echo "openssl is required" >&2; exit 1; }
if [ "$PRINT_ONLY" -eq 0 ]; then
	command -v kubectl >/dev/null || { echo "kubectl is required (or use --print)" >&2; exit 1; }
fi

# ── required external values ────────────────────────────────────────────────
missing=""
for v in PG_HOST MINIO_ACCESS_KEY MINIO_SECRET_KEY S3_ENDPOINT S3_ACCESS_KEY S3_SECRET_KEY S3_BUCKET; do
	eval "val=\${$v:-}"
	[ -n "$val" ] || missing="$missing $v"
done
if [ -n "$missing" ]; then
	echo "error: missing required env vars:$missing" >&2
	echo "export them (see header) and re-run." >&2
	exit 1
fi

PG_PORT="${PG_PORT:-6432}"
PG_USER="${PG_USER:-banya}"
PG_SSLMODE="${PG_SSLMODE:-verify-full}"

# ── generated secrets ───────────────────────────────────────────────────────
# hex (URL-safe) so values never break the postgres:// DSN or header parsing.
gen() { openssl rand -hex "${1:-24}"; }
PG_PASSWORD="$(gen 24)"
INTERNAL_SERVICE_TOKEN="$(gen 32)"
CURSOR_HMAC_KEY="$(gen 32)"

workdir="$(mktemp -d)"
chmod 700 "$workdir"
trap 'rm -rf "$workdir"' EXIT
openssl ecparam -name prime256v1 -genkey -noout -out "$workdir/jwt_ec_priv.pem" 2>/dev/null
openssl ec -in "$workdir/jwt_ec_priv.pem" -pubout -out "$workdir/jwt_ec_pub.pem" 2>/dev/null

# ── emit a secret: apply_secret NAME [lit KEY VALUE]... [file KEY PATH]... ──
# Optional literals with an empty VALUE are skipped (key omitted from secret).
apply_secret() {
	name="$1"; shift
	set -- "$@"
	args=()
	while [ $# -gt 0 ]; do
		kind="$1"; key="$2"; val="$3"; shift 3
		case "$kind" in
			lit)  [ -n "$val" ] && args+=("--from-literal=${key}=${val}") ;;
			file) args+=("--from-file=${key}=${val}") ;;
		esac
	done
	kubectl create secret generic "$name" \
		--namespace "$NAMESPACE" \
		"${args[@]}" \
		--dry-run=client -o yaml \
	| if [ "$PRINT_ONLY" -eq 1 ]; then cat; echo "---"; else kubectl apply -f -; fi
}

# DB-only services
for s in user venue booking review master crm analytics; do
	apply_secret "${s}-service-env" lit PG_PASSWORD "$PG_PASSWORD"
done

# auth: PG + JWT signing keypair + optional mail/telegram
apply_secret auth-service-env \
	lit PG_PASSWORD "$PG_PASSWORD" \
	file JWT_EC_PRIVATE_KEY "$workdir/jwt_ec_priv.pem" \
	file JWT_EC_PUBLIC_KEY "$workdir/jwt_ec_pub.pem" \
	lit SMTP_USER "${SMTP_USER:-}" \
	lit SMTP_PASSWORD "${SMTP_PASSWORD:-}" \
	lit TELEGRAM_BOT_TOKEN "${TELEGRAM_BOT_TOKEN:-}" \
	lit TELEGRAM_CHAT_ID "${TELEGRAM_CHAT_ID:-}"

# realtime services need the internal service token
apply_secret chat-service-env \
	lit PG_PASSWORD "$PG_PASSWORD" \
	lit INTERNAL_SERVICE_TOKEN "$INTERNAL_SERVICE_TOKEN"
apply_secret notification-service-env \
	lit PG_PASSWORD "$PG_PASSWORD" \
	lit INTERNAL_SERVICE_TOKEN "$INTERNAL_SERVICE_TOKEN"

# payment: PG password only — the default mock provider needs no credentials.
# Add the chosen acquiring bank's credentials here once a real provider is wired up.
apply_secret payment-service-env \
	lit PG_PASSWORD "$PG_PASSWORD"

# api-gateway: the widest surface
apply_secret api-gateway-env \
	lit PG_PASSWORD "$PG_PASSWORD" \
	lit INTERNAL_SERVICE_TOKEN "$INTERNAL_SERVICE_TOKEN" \
	lit CURSOR_HMAC_KEY "$CURSOR_HMAC_KEY" \
	file JWT_EC_PUBLIC_KEY "$workdir/jwt_ec_pub.pem" \
	lit MINIO_ACCESS_KEY "$MINIO_ACCESS_KEY" \
	lit MINIO_SECRET_KEY "$MINIO_SECRET_KEY" \
	lit VK_CLIENT_ID "${VK_CLIENT_ID:-}" \
	lit VK_CLIENT_SECRET "${VK_CLIENT_SECRET:-}" \
	lit SMTP_USER "${SMTP_USER:-}" \
	lit SMTP_PASSWORD "${SMTP_PASSWORD:-}" \
	lit SUPPORT_HELPDESK_WEBHOOK_TOKEN "${SUPPORT_HELPDESK_WEBHOOK_TOKEN:-}"

# migrator (PG_* via pkg/config) and pg-backup (libpq PG* names + S3)
apply_secret migrator-env \
	lit PG_HOST "$PG_HOST" lit PG_PORT "$PG_PORT" lit PG_USER "$PG_USER" \
	lit PG_PASSWORD "$PG_PASSWORD" lit PG_SSLMODE "$PG_SSLMODE"
apply_secret pg-backup-env \
	lit PGHOST "$PG_HOST" lit PGPORT "$PG_PORT" lit PGUSER "$PG_USER" \
	lit PGPASSWORD "$PG_PASSWORD" lit PGSSLMODE "$PG_SSLMODE" \
	lit S3_ENDPOINT "$S3_ENDPOINT" lit S3_ACCESS_KEY "$S3_ACCESS_KEY" \
	lit S3_SECRET_KEY "$S3_SECRET_KEY" lit S3_BUCKET "$S3_BUCKET"

echo "" >&2
echo "Done. Generated PG_PASSWORD must also be set on the managed Postgres role" >&2
echo "'${PG_USER}'. Save it now (shown once):" >&2
echo "  PG_PASSWORD=${PG_PASSWORD}" >&2
if [ "$PRINT_ONLY" -eq 0 ]; then
	echo "Secrets applied to namespace '${NAMESPACE}'." >&2
fi
