# Secrets

Production secrets live **only** in Kubernetes Secrets, never in git or
`deploy/.env` (that file is dev-only). Each service's Deployment pulls its
sensitive values from a `<service>-env` Secret via `envFromSecrets`; non-secret
config (ports, DB names, service addresses, endpoints) lives in
`deploy/helm/agregator/values.yaml` under `global.env` and each service's `env:`.

## Generate & apply

[generate-secrets.sh](generate-secrets.sh) generates strong random values and
creates every `*-env` Secret. It generates `PG_PASSWORD`,
`INTERNAL_SERVICE_TOKEN`, `CURSOR_HMAC_KEY`, and the ES256 JWT keypair; you
supply third-party / cloud values via the environment.

```bash
export PG_HOST=<cluster>.mdb.yandexcloud.net
export MINIO_ACCESS_KEY=… MINIO_SECRET_KEY=…          # Object Storage static key
export S3_ENDPOINT=https://storage.yandexcloud.net \
       S3_ACCESS_KEY=… S3_SECRET_KEY=… S3_BUCKET=…    # backup bucket
# optional: VK_CLIENT_ID/SECRET, SMTP_USER/PASSWORD, TELEGRAM_*, SUPPORT_HELPDESK_WEBHOOK_TOKEN

# inspect first (prints plaintext manifests, no cluster needed):
./generate-secrets.sh --print | less
# then apply:
./generate-secrets.sh -n agregator
```

The script prints the generated `PG_PASSWORD` once — **set the same value on the
managed Postgres role** (`banya`) via the YC console, or migrations and every
service will fail to authenticate.

## Secret inventory

| Secret | Keys |
|---|---|
| `auth-service-env` | `PG_PASSWORD`, `JWT_EC_PRIVATE_KEY`, `JWT_EC_PUBLIC_KEY`, `SMTP_USER?`, `SMTP_PASSWORD?`, `TELEGRAM_BOT_TOKEN?`, `TELEGRAM_CHAT_ID?` |
| `api-gateway-env` | `PG_PASSWORD`, `INTERNAL_SERVICE_TOKEN`, `CURSOR_HMAC_KEY`, `JWT_EC_PUBLIC_KEY`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `VK_*?`, `SMTP_*?`, `SUPPORT_HELPDESK_WEBHOOK_TOKEN?` |
| `chat-service-env`, `notification-service-env` | `PG_PASSWORD`, `INTERNAL_SERVICE_TOKEN` |
| `payment-service-env` | `PG_PASSWORD` (acquiring-gateway creds added when a real provider is wired up) |
| `user/venue/booking/review/master/crm/analytics-service-env` | `PG_PASSWORD` |
| `migrator-env` | `PG_HOST`, `PG_PORT`, `PG_USER`, `PG_PASSWORD`, `PG_SSLMODE` |
| `pg-backup-env` | `PGHOST`, `PGPORT`, `PGUSER`, `PGPASSWORD`, `PGSSLMODE`, `S3_*` |

`?` = included only if you supply it; add later with
`kubectl create secret … --dry-run=client -o yaml | kubectl apply -f -`.

## Notes

- **`JWT_SECRET` is gone.** The gateway verifies ES256 (`JWT_EC_PUBLIC_KEY`); the
  old symmetric `JWT_SECRET` is unused — do not carry it into prod.
- **Rotation.** Re-run the script to roll generated secrets, then restart the
  affected Deployments (`kubectl rollout restart`). Rotating `PG_PASSWORD` also
  requires updating the managed Postgres role in the same change window. Rotate
  the JWT keypair per [docs/jwt-key-rotation.md](../../../docs/jwt-key-rotation.md).
- **Grafana** (`GRAFANA_ADMIN_PASSWORD`, `GRAFANA_RO_PASSWORD`) is deployed by
  the observability compose, not this chart — rotate those in that stack's env.
- For GitOps (encrypted secrets in git) migrate later to Sealed Secrets or
  External Secrets + Yandex Lockbox; this script is the plain-kubectl baseline.
