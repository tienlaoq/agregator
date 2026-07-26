# agregator — Baths & Saunas Aggregator

Microservices platform: online booking, master profiles, goods shop, certifications.

## Commands

```bash
# Infrastructure
make infra-up / infra-down / infra-reset

# Build & run
make build           # all services → bin/
make docker-up       # all services in containers
make run-<service>   # dev with live reload: auth|user|venue|booking|review|payment|master|gateway
make run-frontend    # Next.js dev server (http://localhost:3000)

# Maintenance
make proto-gen       # regenerate Go from .proto (run after ANY .proto change)
make tidy            # go mod tidy on all modules
make test            # all service tests
make migrate         # run DB migrations
make lint            # golangci-lint + ESLint
```

## Architecture

8 microservices + 1 API gateway. Each service in `services/{name}/` follows Clean Architecture:

```
cmd/ → internal/{domain, usecase, repository, delivery/grpc}/ → migrations/
```

| Layer | Role |
|---|---|
| domain | Entities + repository interfaces |
| usecase | Business logic, orchestration |
| repository | DB queries |
| delivery/grpc | Proto ↔ domain marshalling, error mapping |

**Sync**: gRPC between services. **Async**: NATS JetStream for events (bookings, payments, reviews).  
**Storage**: PostgreSQL per service (PostGIS in venue), Redis, MinIO, Meilisearch.  
**Go Workspace**: `go.work` covers `gen/go`, `pkg`, `services/*`.

## Frontend

Next.js 16 + React 19 in `frontend/`. App Router, Server Components, TanStack Query, Zustand, Tailwind v4.  
SSR is critical — SEO keywords like "баня рядом [город]".

Route structure: `src/app/(public)/` — public pages with AppLayout (header + footer + chat widget).  
`src/app/admin/`, `src/app/owner/`, `src/app/auth/` — isolated layouts without public navigation.  
Feature modules: `src/features/chat/` (WebSocket chat, ws-ticket auth).

```bash
cd frontend
npm run dev          # dev server
npm run test:run     # Vitest (unit)
npm run test:e2e     # Playwright (e2e)
```

## Handler structure

`services/api-gateway/internal/handler/` — один пакет, ~30 файлов. Файлы уже
разделены по доменам (`venue_photos.go`, `venue_crm.go`, `chat_thread_resolver.go`
и т.д.). При добавлении нового домена (goods, notifications, search) — создавай
отдельный файл `{domain}.go` в том же пакете.

Общие HTTP-хелперы (`WriteJSON`, `ReadJSONOrRespond`, `QueryInt`, `WriteCatalog`,
`GRPCErrorToHTTP`) вынесены в leaf-пакет `internal/httpx` — он зависит только от
`apicatalog` + `limits` и не импортирует `handler`. Это снимает главную ловушку
будущей разбивки на подпакеты (раньше — циклический импорт через `response.go`,
теперь файл удалён): доменные подпакеты могут импортировать `httpx` без цикла
назад в корень `handler`. Новые домен-агностичные хелперы клади в `httpx`,
домен-специфичные — в файл своего домена.

## Rules

- Run `make proto-gen` after any `.proto` change before building
- Run `make infra-up` + `bash deploy/migrate.sh` before first local run
- Env vars from `deploy/.env` (git-ignored); see `deploy/.env.example`
- Custom error types in `internal/domain/`; map to gRPC status codes in `delivery/grpc/`
- Migrations: numbered SQL files `001_*.sql`, `002_*.sql` in each service's `migrations/`
- Shared packages go in `pkg/`; import as `github.com/banya-io/pkg/...`
- Never commit secrets; use `CLAUDE.local.md` for personal sandbox URLs/keys

## Webhook logging policy

Applies to every log statement in the webhook request path
(`api-gateway/internal/handler/payment.go` → `payment-service/.../grpc/server.go`).

**Permitted** — safe structural metadata, no PII risk:

| Field | Where |
|---|---|
| `body_bytes` | byte length of the received body — size, not content |
| `content_type` | Content-Type header |
| `client_ip` | source IP; already logged on rejection |
| `event` | notification type, e.g. `"payment.succeeded"` |
| `object_id` | provider payment UUID |
| `object_status` | status string from payload (`"succeeded"`, `"canceled"`) |
| `err` | error values on failure paths |

**Never log** — PII or financial data that the payment provider may expand without notice:

- `body` / `raw_body` / any fragment of the HTTP request body
- `amount`, `value`, `currency`
- `metadata.*` — may carry booking ID, user details, or custom fields
- `payer`, `recipient`, `description`
- Any field not in the Permitted list above

The exhaustive lists live as comments in the source (`webhookBodyLog` in
`handler/payment.go` and the `HandleWebhook` doc-comment in `server.go`).
Adding a new log field requires an explicit PII review and an update to both
the source comment and this table.

<!-- maintainer: path-specific rules live in .claude/rules/ — keep this file under 200 lines -->
