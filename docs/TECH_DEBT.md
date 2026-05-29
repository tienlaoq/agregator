# Tech Debt Register

Entries are tagged `[AREA-NNN]` and referenced from source comments.
Each entry describes: the current behaviour, the accepted risk, and the
concrete upgrade path when the requirement changes.

---

## ~~[BOOKING-ORPHAN]~~ — ЗАКРЫТО

**Закрыто**: reaper реализован в `AutoCompletePastVisits` (шаг 4).  
`repo.DeleteOrphanPending(ctx, 15)` вызывается каждые 2 минуты тикером в `cmd/main.go`.  
При срабатывании пишет `WARN` с `count` — сигнал для расследования первопричины.

~~Зависшая бронь при сбое repo.Delete после отказа ReserveSlot или SetPaymentAndStatus~~

---

## [BOOKING-ORPHAN-PAYMENT] — Осиротевший платёж при сбое repo.SetPaymentAndStatus

**File**: `services/booking-service/internal/usecase/booking.go` — `CreateBooking()`  
**Proto**: `proto/payment/v1/payment.proto`

**Current behaviour**  
Если `repo.SetPaymentAndStatus` упадёт после успешного `CreatePayment` —
платёж в payment-service уже создан и выставлен пользователю, но запись в
`bookings` остаётся в статусе `pending` без `payment_id`. Платёж нельзя
отозвать программно: `CancelPayment` RPC отсутствует в proto.

Компенсационная ветка делает `ReleaseSlot` + `repo.Delete` (удаляет `pending`-
запись), но платёж при этом продолжает жить в payment-service. Пользователь
видит ссылку на оплату (если YooKassa успела её выдать) без соответствующей
брони.

**Accepted residual risk**  
Вероятность крайне мала: сбой одного UPDATE по PK после успешного gRPC-вызова.
Практически возможен только при crash PG-ноды в этом точном окне. Осиротевший
платёж истечёт по TTL на стороне YooKassa (обычно 1 час для pending-платежей)
и не спишет деньги. Тем не менее это UX-аномалия: пользователь видит ссылку
на оплату, переходит, оплачивает — webhook `payment.succeeded` придёт, но
`ConfirmBooking` найдёт запись уже удалённой (или отсутствующей) и вернёт
`NotFound`.

**Upgrade path**  
1. Добавить `rpc CancelPayment(CancelPaymentRequest) returns (CancelPaymentResponse)`
   в `proto/payment/v1/payment.proto`.
2. Реализовать в payment-service: отзыв платежа через YooKassa API
   (`POST /payments/{id}/cancel`).
3. В `CreateBooking`, при сбое `SetPaymentAndStatus`, вызывать `CancelPayment`
   до `repo.Delete`.

**Triggers for prioritisation**  
- Появление жалоб на «оплатил, но брони нет» (webhook от YooKassa по
  осиротевшему платежу).
- Добавление `CancelPayment` уже нужно для других флоу (ручная отмена
  payment_pending-брони администратором).

---

## [BOOKING-STALE-HALL-IDS] — HallIDs в истории броней могут ссылаться на удалённые залы

**File**: `services/booking-service/internal/usecase/booking.go` — `effectiveHourlyPriceFromVenueAndHalls()`  
**Affected**: frontend CRM (`/owner/venues/{id}/bookings`), `BookingResponse.hall_ids`

**Current behaviour**  
При создании брони `validateHallIDs` проверяет принадлежность зала к venue.
Если после создания владелец удаляет зал из venue-service, исторические брони
сохраняют `booking_hall_ids = ['<deleted-uuid>']`. `GetBooking` и `ListVenueBookings`
возвращают эти IDs as-is через `toProto`. Фронт CRM делает резолв зала по UUID —
получает 404 и рендерит «зал не найден».

**Почему backend не должен это фильтровать**  
`HallIDs` в брони — исторический факт: пользователь бронировал конкретный зал,
он существовал на момент оплаты. Фильтрация на backend изменила бы исторические
данные и сломала бы `total_price` (он рассчитан с учётом price_from этого зала).
Удалённый зал должен оставаться в записи; правильная реакция — на стороне UI.

**Accepted residual risk**  
CRM показывает "зал не найден" для броней с удалёнными залами. Это редкий
сценарий (зал удаляется уже после совершённых броней). Финансовые данные
(total_price) корректны; проблема исключительно UX.

**Fix — frontend**  
В CRM-компоненте отображения зала: если резолв UUID возвращает 404, показывать
`«Зал удалён (ID: {short-uuid})»` вместо ошибки. Пример:
```tsx
const hallName = hallMap[hallId]?.name ?? `Зал удалён (${hallId.slice(0, 8)}…)`;
```

**Fix — backend (опционально, для API-чистоты)**  
Добавить в `BookingResponse` поле `hall_names: map<string, string>` —
снапшот имён залов на момент создания брони. Хранить в отдельной колонке
`hall_names_snapshot JSONB`. Тогда фронт не зависит от актуального состояния
venue-service.

**Triggers for prioritisation**  
- Жалобы владельцев на «сломанный» CRM после реструктуризации залов.
- Бизнес-требование хранить полный аудит-трейл (имя зала, цена) на момент брони.

---

## [BOOKING-PUBLISH-LOSS] — NATS publish без транзакционного outbox

**File**: `services/booking-service/internal/usecase/booking.go` — `publishEvent()`

**Current behaviour**  
Четыре события (`booking.created`, `booking.cancelled` ×2, `booking.confirmed`)
публикуются через `publishEvent()` после успешной операции с БД. При недоступном
NATS ошибка логируется (`log.Error`), но не возвращается вызывающему — операция
уже совершена и откатить её нельзя. Downstream-консьюмеры (review, notification)
не получают триггер.

Исключение: `booking.completed` в `AutoCompletePastVisits` уже частично защищён
catch-up паттерном через `completed_event_sent_at` (см. миграцию 007).

**Accepted residual risk**  
При кратковременном сбое NATS событие теряется навсегда. Для `booking.created`
это некритично (бронь видна в UI через polling). Для `booking.cancelled` —
рефанд в payment-service не инициируется автоматически, потребуется ручное
вмешательство. Для `booking.confirmed` — notification-сервис не отправит
подтверждение пользователю.

**Upgrade path — transactional outbox**  
1. Создать таблицу:
   ```sql
   CREATE TABLE outbox_events (
       id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       subject    TEXT        NOT NULL,
       payload    JSONB       NOT NULL,
       created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
       sent_at    TIMESTAMPTZ
   );
   CREATE INDEX idx_outbox_unsent ON outbox_events (created_at)
       WHERE sent_at IS NULL;
   ```
2. В `CreateBooking`, `CancelBooking`, `ConfirmBooking` — INSERT в `outbox_events`
   в той же транзакции что и основная операция (потребует транзакций в репозитории).
3. Отдельный поллер (уже есть тик в `cmd/main.go` для `AutoCompletePastVisits`)
   читает `WHERE sent_at IS NULL ORDER BY created_at LIMIT 100`, публикует,
   обновляет `sent_at = now()`.
4. Убрать прямые вызовы `publisher.Publish*` из usecase; оставить только в поллере.

**Triggers for prioritisation**  
- SLA на уведомления о подтверждении/отмене брони.
- Жалобы на отсутствие рефанда при отмене.
- Рост нагрузки → NATS restarts → видимые потери событий в логах
  (`NATS publish failed — event lost`).

---

## [AUTH-DENYLIST] — Access-token invalidation on logout

**File**: `services/auth-service/internal/usecase/auth.go` — `Logout()`

**Current behaviour**  
`Logout` revokes the refresh token (DB delete) but does *not* invalidate the
corresponding access token. The access JWT remains valid until its `exp` claim
elapses (default `JWT_ACCESS_TTL = 15 min`).

**Why this is intentional**  
Access tokens are stateless ES256 JWTs verified at the gateway without any
DB or cache lookup. Keeping the hot-path stateless is the main scalability
and latency advantage of the current architecture. A denylist would add a
Redis GET to every authenticated request.

**Accepted residual risk**  
After logout the old access token remains usable for up to `accessTTL`.
Clients MUST discard the token on logout so it never leaves the device.
Combined with short TTL + HTTPS + secure/httpOnly cookie storage, the
practical attack surface is narrow (requires active token theft during the
window).

**Upgrade path (when needed)**  
1. Add a `jti` (JWT ID) field to `pkg/auth.Claims` and populate it in
   `auth.GenerateAccessToken` with a `uuid.New().String()`.
2. On `Logout`, write `SET auth:denylist:{jti} 1 EX {remaining_ttl}` to
   Redis. The gateway already connects to Redis (`REDIS_ADDR`).
3. In the gateway JWT middleware, after signature verification, do
   `EXISTS auth:denylist:{jti}` and return `401` on a hit.
4. On forced logout (account deletion, password change, admin revocation),
   call `Logout` for every active session returned by
   `tokens.ListByUserID` (new repo method needed).

**Triggers for prioritisation**  
- Admin/high-privilege role accounts (consider shorter `accessTTL` as a
  cheaper interim measure — set `JWT_ACCESS_TTL=5m` for admin tokens).
- Regulatory requirements (HIPAA, PCI-DSS) that mandate immediate session
  termination.
- Password-change flow: currently an attacker who steals an access token
  retains access for up to `accessTTL` even after the victim changes their
  password. A denylist on password change would close this window.

---

## [STORAGE-01] — Single-replica disk uploader in dev mode

**File**: `services/api-gateway/cmd/config.go` — `MinIOEndpoint`

**Current behaviour**  
When `MINIO_ENDPOINT` is empty the gateway falls back to `DiskUploader`,
writing uploaded files to the local filesystem (`UPLOAD_ROOT`). This works
for single-replica local development but is incompatible with horizontal
scaling or container restarts that do not preserve the volume.

**Accepted residual risk**  
Dev/staging data loss on container restart. No production risk as long as
`MINIO_ENDPOINT` is always set in production deploys (enforced by
`deploy/.env.example`).

**Upgrade path**  
Set `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`,
`MINIO_BUCKET`, and `MINIO_PUBLIC_BASE_URL` in `deploy/.env` and restart.
No code changes required. Remove `DiskUploader` entirely once all
environments are confirmed to use MinIO.

---

## [AUTH-REFRESH-RETRY] — RefreshToken is not idempotent; network retries force re-login

**File**: `services/auth-service/internal/usecase/auth.go` — `RefreshToken()`

**Current behaviour**  
`ConsumeByHash` (DELETE … RETURNING) is the first operation in `RefreshToken`.
Once the row is deleted, the old token is gone. If the network drops between
the DELETE and the client receiving the response, a retry with the same raw
token hits NotFound → `Unauthenticated`. The client is forced into full
re-authentication (password or OAuth flow) even though the server issued a
valid new pair.

This is an intentional trade-off: DELETE-first eliminates the userID-existence
leak that a SELECT-then-DELETE pattern would create. The cost is that a single
unanswered request destroys the token irrecoverably.

**Accepted residual risk**  
Mobile clients on flaky connections (LTE handovers, airplane mode transitions)
will occasionally need to re-authenticate. The UX impact is a login prompt
that should not normally appear mid-session. This is considered acceptable
given that:

- Access tokens last `JWT_ACCESS_TTL` (default 15 min), so the window where a
  refresh is strictly necessary is short.
- Clients that store the new pair immediately on receipt and do not retry on
  timeout (see client-side mitigation below) are not affected.

**Client-side mitigation (no server changes needed)**  
1. Persist the new `{access_token, refresh_token}` pair to durable storage
   the moment the response is received, before any other network activity.
2. On a refresh call that times out or returns a connection error, do **not**
   retry automatically. Treat it as a forced re-login and prompt the user.
   (A retry on timeout is indistinguishable from a duplicate request to the
   server; it will always get `Unauthenticated`.)
3. Use a short initial timeout (≤ 5 s) with no automatic retry for the refresh
   call itself.

**Server-side upgrade path (idempotency via tombstone)**  
1. Add migration:
   ```sql
   CREATE TABLE consumed_refresh_tokens (
       old_token_hash TEXT        PRIMARY KEY,
       new_token_hash TEXT        NOT NULL,
       user_id        TEXT        NOT NULL,
       consumed_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
       expires_at     TIMESTAMPTZ NOT NULL  -- same TTL as the original token
   );
   CREATE INDEX ON consumed_refresh_tokens (expires_at); -- cleanup
   ```
2. Wrap the existing `ConsumeByHash` DELETE and a new INSERT into `consumed_refresh_tokens`
   in a single transaction. The new token pair must be generated *after* the transaction
   commits (or store `new_token_hash` in the tombstone within the same TX using a
   two-phase approach: reserve → generate → commit).
3. On `ConsumeByHash` returning NotFound, query `consumed_refresh_tokens` by
   `old_token_hash`. If a row exists and `consumed_at > now() - graceWindow`
   (suggested: 30 s), re-issue the stored `new_token_hash` as the response.
   If outside the grace window, return `Unauthenticated` as today.
4. Add `consumed_refresh_tokens` cleanup to `runTokenCleanup`:
   `DELETE FROM consumed_refresh_tokens WHERE expires_at < now()`.
5. Add `GetConsumedByOldHash(ctx, oldHash) (*ConsumedToken, error)` to
   `domain.RefreshTokenRepository` and implement in the repo layer.

**Security note on the tombstone approach**  
The tombstone must be written in the **same transaction** as the DELETE, or
between the DELETE and the response — never before. Writing it before the
DELETE would re-introduce the window where two concurrent requests both see
the row as "not yet consumed". The grace window (30 s) must be short enough
to be useless for an attacker who has stolen a token but long enough to cover
legitimate network retries (typically < 5 s RTT).

**Triggers for prioritisation**  
- User complaints about unexpected logout on mobile (LTE/WiFi transitions).
- Analytics showing elevated re-authentication rates on mobile clients.
- Requirement to support long-lived sessions (e.g. "stay logged in for 30 days")
  where a forced re-login is a significant UX regression.

---

## [AUTH-REUSE-TOMBSTONE] — Delayed refresh-token replay not detected

**File**: `services/auth-service/internal/usecase/auth.go` — `RefreshToken()`  
**Repo**: `services/auth-service/internal/repository/postgres.go` — `ConsumeByHash()`

**Current behaviour**  
`RefreshToken` calls `ConsumeByHash` (DELETE … RETURNING) as the *first*
operation, before any identity fetch. This eliminates the userID-existence
leak that a SELECT-then-DELETE pattern would create, and prevents the
concurrent-replay race (two simultaneous requests with the same token: only
one DELETE wins, the other gets NotFound → Unauthenticated immediately).

However, it does **not** detect a *delayed* replay:

1. Attacker steals token *T* from the legitimate owner.
2. Attacker calls `RefreshToken(T)` → `ConsumeByHash` succeeds → attacker
   receives a fresh token pair *T′*.
3. Owner later calls `RefreshToken(T)` → `ConsumeByHash` returns NotFound
   (row gone) → owner receives `Unauthenticated` — indistinguishable from
   an expired token.
4. The attacker's pair *T′* is **not** revoked.

**Accepted residual risk**  
An attacker who successfully steals and replays a token before the legitimate
owner retries keeps the stolen session. The window is bounded by
`JWT_REFRESH_TTL` (default 7 days). The attack requires active token theft
(man-in-the-middle, device compromise, XSS against httpOnly bypass — all
already mitigated at the transport/browser layer).

**Upgrade path (when needed)**  
Add a `consumed_refresh_tokens` table as a tombstone store:

```sql
CREATE TABLE consumed_refresh_tokens (
    token_hash  TEXT        PRIMARY KEY,
    user_id     TEXT        NOT NULL,
    consumed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL  -- same TTL as the live token
);
CREATE INDEX ON consumed_refresh_tokens (expires_at); -- for TTL cleanup
```

In `ConsumeByHash`, wrap the DELETE and tombstone INSERT in a single
transaction:

```sql
WITH deleted AS (
    DELETE FROM refresh_tokens WHERE token_hash = $1 RETURNING *
)
INSERT INTO consumed_refresh_tokens (token_hash, user_id, expires_at)
SELECT token_hash, user_id, expires_at FROM deleted;
```

In `RefreshToken` (usecase), after `ConsumeByHash` returns NotFound, query
the tombstone table. If the hash is found there, the token was previously
consumed → treat as a replay: call `tokens.DeleteByUserID` to revoke all
active sessions for that `user_id`, log a Warn, and return Unauthenticated
with a distinct message.

A background cleanup job (extend the existing `runTokenCleanup`) should
`DELETE FROM consumed_refresh_tokens WHERE expires_at < now()` on the same
schedule.

**Triggers for prioritisation**  
- Evidence of token theft in production logs (owner retries failing
  immediately after a successful consume from a different IP/UA).
- Compliance requirements mandating session-anomaly detection.
- Introduction of long-lived tokens (e.g. "remember me" with 30-day TTL)
  where the delayed-replay window becomes operationally significant.

---

## [AUTH-OAUTH-ORPHAN] — Orphan OAuth accounts from unverified-email providers

**File**: `services/auth-service/internal/usecase/auth.go` — `OAuthLogin()` branch 3  
**Repo**: `services/auth-service/internal/repository/postgres.go` — `DeleteOrphanOAuthAccounts()`

**Current behaviour**  
When a provider returns `EmailVerified=false`, `OAuthLogin` creates a new
user account with an empty email (`credentials.email = ''`,
`password_hash IS NULL`). This prevents account-takeover via unverified email
linking. However the created account is an "orphan": it has no email, no
password, and no way for the user to recover it via password reset.

Two mitigations are implemented:

1. **Email promotion** (branch 1): if the same `provider/provider_id` returns
   later with `EmailVerified=true`, `PromoteOAuthEmail` backfills the email on
   the credential row, converting the orphan into a full account. The promotion
   is best-effort — failures are logged but do not block login.

2. **Cleanup job** (`runTokenCleanup`): orphan credentials older than
   `OAUTH_ORPHAN_MIN_AGE` (default 30 days) with no active refresh tokens are
   deleted automatically. This bounds table growth even if the user never
   returns.

**Remaining limitation: user-service not updated on promotion**  
When `PromoteOAuthEmail` updates `credentials.email`, the corresponding record
in user-service is NOT updated. user-service stores its own email copy and
currently has no `UpdateEmail` RPC. Consequence: JWT claims issued after
promotion carry the correct email (from the credential row), but user-service
profile queries still return an empty email until an `UpdateEmail` RPC is
added and wired up.

**Upgrade path**  
1. Add `UpdateUserEmail(userID, email string)` to the user-service proto.
2. Add `UpdateEmail(ctx, userID, email string) error` to `domain.UserClient`
   and implement in `adapter.UserClientAdapter`.
3. In `OAuthLogin` branch 1, after a successful `PromoteOAuthEmail`, call
   `uc.userClient.UpdateEmail(ctx, cred.UserID, in.Email)` — also best-effort
   (log on failure, do not block login).

**Triggers for prioritisation**  
- User-service profile pages showing empty email for OAuth users who have been
  promoted.
- GDPR/account-management requirements for consistent email across services.

---

## [AUTH-OAUTH-RACE-ORPHAN] — Orphan user-service record on concurrent new-user OAuth race

**File**: `services/auth-service/internal/usecase/auth.go` — `OAuthLogin()` branch 3

**Current behaviour**  
When two requests arrive simultaneously for the same `provider/providerID` and
neither finds an existing credential (both pass branch 1), both proceed to
branch 3 and call `userClient.CreateUser` with different UUIDs. One
`CreateOAuth` INSERT wins; the other hits the UNIQUE constraint on
`(provider, provider_id)`. The losing request recovers by fetching the
winning credential and issuing tokens for that account. However, the user-
service record created by the losing request (a different UUID) is now an
orphan — it has no credential on the auth-service side and is unreachable
by any subsequent flow.

A Warn log is emitted with `orphan_user_id` to make these visible:

```
oauth: concurrent new-user race — credential already inserted by peer;
orphan user-service record may exist for orphan_user_id
```

**Accepted residual risk**  
Orphan user-service records accumulate over time. They are inert (no
associated credentials, no way to log in as them), but they consume storage
and may confuse admin tooling. The race is inherently rare — it requires two
simultaneous first-logins for the same provider account, which is unlikely in
normal operation and essentially impossible without a network anomaly or
deliberate load testing.

**Upgrade path**  
1. Add `DeleteUser(ctx context.Context, userID string) error` to the user-
   service proto and `domain.UserClient` interface.
2. In `OAuthLogin` branch 3, after detecting `AlreadyExists` on `CreateOAuth`,
   call `uc.userClient.DeleteUser(ctx, userResp.ID)` as a best-effort
   compensating action (log on failure, do not block the recovery path).
3. Alternatively, make `CreateOAuth` an upsert
   (`INSERT … ON CONFLICT DO NOTHING RETURNING …`) combined with a
   `SELECT WHERE provider = $1 AND provider_id = $2` fallback, eliminating
   the race window without requiring a user-service deletion RPC.

**Triggers for prioritisation**  
- Orphan records appearing in production user-service DB (query:
  `SELECT id FROM users u WHERE NOT EXISTS (SELECT 1 FROM credentials c WHERE c.user_id = u.id::text)`).
- Load testing revealing elevated orphan counts under concurrent OAuth traffic.

---

## [AUTH-OAUTH-AVATAR] — OAuth avatar URL not persisted on registration

**File**: `services/auth-service/internal/usecase/auth.go` — `OAuthInput.AvatarURL`  
**Proto**: `proto/auth/v1/auth.proto` — `OAuthLoginRequest.avatar_url`

**Current behaviour**  
`OAuthLoginRequest` carries `avatar_url` (field 5), which delivery/grpc copies
into `OAuthInput.AvatarURL`. The field is accepted at the transport boundary
so it is not silently dropped, but it is never forwarded to user-service.
`CreateUserRequest` in `user.proto` has no `avatar_url` field, so the value
cannot be passed through `CreateUserInput → UserClientAdapter → user-service`
on new-user registration (branch 3 of `OAuthLogin`).

For returning users (branches 1 and 2) the avatar is even further out of
reach: user-service would need an `UpdateUser` call, which exists but is not
called by auth-service.

**Accepted residual risk**  
New OAuth users have no avatar in user-service immediately after registration.
Clients work around this today by calling `UpdateUser` (via api-gateway) with
the avatar URL immediately after a successful `OAuthLogin`. This adds a
round-trip but is functional.

**Upgrade path**  
1. Add `string avatar_url = 6` to `CreateUserRequest` in `proto/user/v1/user.proto`.
2. Run `make proto-gen`.
3. Add `AvatarURL string` to `domain.CreateUserInput` (auth-service entity).
4. In `OAuthLogin` branch 3, populate `CreateUserInput.AvatarURL = in.AvatarURL`.
5. In `adapter.UserClientAdapter.CreateUser`, map `in.AvatarURL → grpcReq.AvatarUrl`.
6. Handle `AvatarURL` in user-service `CreateUser` usecase and repository.

**Triggers for prioritisation**  
- Product requirement to show correct OAuth avatar immediately after first login,
  without an extra client-side `UpdateUser` call.

---

## [MASTER-FTS-MEILI] — ListPublic: PostgreSQL tsvector вместо Meilisearch

**File**: `services/master-service/internal/repository/postgres.go` — `ListPublic()`  
**Migration**: `services/master-service/migrations/011_masters_fts.up.sql`

**Current behaviour**  
Текстовый поиск по публичному каталогу мастеров использует PostgreSQL
`tsvector @@ plainto_tsquery('russian', …)` с GIN-индексом `idx_masters_fts`
покрывающим `display_name || bio || city || specializations`.  
Slug и phone по-прежнему фильтруются через `ILIKE '%q%'` (нет B-tree покрытия
по leading wildcard — приемлемо, т.к. это вторичные условия через `OR`).  
Поиск по `master_services.name/description` — через коррелированный `EXISTS`
с `ILIKE` fallback (кандидаты уже отфильтрованы tsvector, множество мало).

Это та же техника, что venue-service (`idx_venues_fts`, migration 001). Meili­search
упомянут в CLAUDE.md как целевая инфраструктура для каталогов, но ни venue-service,
ни master-service к нему не подключены.

**Accepted residual risk**  
- Typo-tolerance и нечёткое совпадение недоступны (`plainto_tsquery` — строгий
  морфологический разбор, опечатки не исправляются).
- Ранжирование по релевантности отсутствует; сортировка только по `display_name ASC`.
- **`EXISTS (SELECT 1 FROM master_services … ILIKE)`** — correlated subquery
  без индекса на `master_services`. Комментарий в коде утверждал, что планировщик
  оценит EXISTS последним (после FTS/ILIKE по masters), но `OR`-ветви не cascading:
  планировщик вправе начать с EXISTS если его cost-estimate ниже. При росте каталога
  это может вылиться в полный скан `master_services` для каждого запроса.  
  **Промежуточное решение**:
  ```sql
  CREATE INDEX idx_master_services_fts
      ON master_services
      USING GIN (to_tsvector('russian', name || ' ' || COALESCE(description, '')))
      WHERE length(name) > 0;
  ```
  После этого EXISTS-subquery с `name @@ plainto_tsquery(...)` вместо `ILIKE` будет
  index-safe. До перехода на Meili — достаточно для каталогов до ~100k услуг.
- **`slug ILIKE '%q%'` и `phone ILIKE '%q%'`** — leading-wildcard, B-Tree индекс
  не используется. При 10k+ активных мастеров это сканирование ~10k строк на
  каждый поисковый запрос. Кандидатный набор уже ограничен `status = 'active'`
  через `idx_masters_status` (частичный индекс), но сам ILIKE остаётся O(N).  
  **Промежуточное решение** (до перехода на Meili): добавить `pg_trgm` GIN-индексы:
  ```sql
  CREATE EXTENSION IF NOT EXISTS pg_trgm;
  CREATE INDEX idx_masters_slug_trgm  ON masters USING GIN (slug  gin_trgm_ops) WHERE status = 'active';
  CREATE INDEX idx_masters_phone_trgm ON masters USING GIN (phone gin_trgm_ops) WHERE status = 'active';
  ```
  После этого `ILIKE '%q%'` будет использовать GIN-индекс автоматически без изменения
  кода. Миграция нужна только при росте каталога за ~5k активных мастеров — до этого
  порога sequential scan по partial index дешевле чем GIN lookup.

**Upgrade path → Meilisearch**  
1. Развернуть Meili (уже есть в `deploy/`-инфраструктуре согласно CLAUDE.md).
2. Добавить `MasterSearchClient` interface в domain:
   ```go
   type MasterSearchClient interface {
       IndexMaster(ctx context.Context, m *Master) error
       DeleteMaster(ctx context.Context, masterID uuid.UUID) error
       Search(ctx context.Context, q string, filters SearchFilters) ([]uuid.UUID, error)
   }
   ```
3. Реализовать в `internal/repository/meili.go`; вызывать `IndexMaster` при
   `Insert`, `UpdateProfile`, `UpdateStatus`; `DeleteMaster` не нужен (мастера
   не удаляются, только деактивируются через статус).
4. В `ListPublic`: если `p.Query != ""` — запросить Meili → получить `[]uuid.UUID` →
   `WHERE m.id = ANY($n::uuid[])` вместо tsvector условия. Фильтры по city,
   work_format, price остаются в Postgres (Meili фильтрует по атрибутам тоже,
   но джойн с сервисами сложнее).
5. Удалить `idx_masters_fts` (migration 012_drop_fts_index.up.sql) после перехода.

**Triggers for prioritisation**  
- Жалобы на качество поиска («ввёл "сауна", не нашёл "Сауна на Ленина"» —
  tsvector должен это покрыть через стемминг; опечатки — нет).
- Каталог превышает ~50k активных мастеров → tsvector + GIN всё ещё O(log N),
  но ранжирование становится востребованным продуктово.
- Venue-service переходит на Meili первым — тогда переиспользовать client-код.

---

## [MASTER-BOOKING-CRON] — confirmed→completed переход не автоматизирован

**File**: `services/master-service/internal/repository/postgres.go` — `HasCompletedBookingByClientMaster()`  
**Migration**: `services/master-service/migrations/020_booking_status_completed.up.sql`

**Current behaviour**  
Для review-gate мастерских бронирований используется `HasCompletedBookingByClientMaster`,
которая (после фикса 3.21) считает booking "завершённым" двумя способами:

1. `status = 'completed'` — явный терминальный статус (migration 020 добавил его в CHECK);
2. `status = 'confirmed' AND (date || ' ' || time_to)::timestamp < now()` — неявное условие:
   слот прошёл, но строка ещё не обновлена.

Второй путь работает без cron-джоба, но статус `confirmed` для прошедших броней остаётся
висящим в БД: `ListBookingsByMaster`/`ListBookingsByClient` показывают их как `confirmed`,
что может вводить пользователя в заблуждение.

**Accepted residual risk**  
Отзывы на мастеров работают end-to-end (неявный путь). Статус в UI показывает `confirmed`
вместо `completed` для прошедших слотов — UX-аномалия, не функциональный баг.

**Upgrade path — cron-джоб AutoCompletePastMasterBookings**  
По аналогии с booking-service `AutoCompleteVisitEnded`:

1. Добавить в `MasterRepository`:
   ```go
   // AutoCompletePastBookings transitions confirmed → completed for all
   // master_bookings where (date || ' ' || time_to)::timestamp < now().
   // Returns the number of rows updated.
   AutoCompletePastBookings(ctx context.Context) (int64, error)
   ```
2. Реализовать в `postgres.go`:
   ```sql
   UPDATE master_bookings
      SET status = 'completed', updated_at = now()
    WHERE status = 'confirmed'
      AND (date::text || ' ' || time_to)::timestamp < now()
   ```
3. В `cmd/main.go` добавить тикер (каждые 15 минут достаточно):
   ```go
   ticker := time.NewTicker(15 * time.Minute)
   go func() {
       for range ticker.C {
           n, err := repo.AutoCompletePastBookings(context.Background())
           if err != nil { log.Error().Err(err).Msg("auto-complete master bookings failed") }
           if n > 0 { log.Info().Int64("count", n).Msg("master bookings auto-completed") }
       }
   }()
   ```
4. После реализации — неявный путь в `HasCompletedBookingByClientMaster` можно
   упростить обратно до `status = 'completed'`, если latency тикера приемлема
   (максимум 15 мин задержки от окончания слота до открытия окна отзыва).

**Triggers for prioritisation**  
- Жалобы пользователей на «статус брони завис в confirmed».
- Требование публиковать `booking.master_completed` event для downstream
  (например, notification-service для post-visit NPS).
- Аналитика по completed-бронированиям (сейчас они не счётны по статусу).

---

## [MASTER-GOD-USECASE] — MasterUseCase god-object (~1000 строк, 6 ответственностей)

**File**: `services/master-service/internal/usecase/master.go`

**Current behaviour**  
`MasterUseCase` объединяет шесть логически различимых ответственностей:
профиль мастера, публичный каталог, модерация, выплатные реквизиты,
бронирования (включая платёжный флоу) и управление фотографиями.
Структура компилируется и тестируется нормально; расхождения между ними
на уровне зависимостей пока нет — все методы читают/пишут через один `repo`.

**Accepted residual risk**  
Мёрж-конфликты при параллельной разработке нескольких фич. Сложность
навигации по файлу по мере роста числа методов.

**Upgrade path**  
При появлении нового домена (CRM, уведомления, аналитика) или при первом
мёрж-конфликте в `master.go` — разбить по одному usecase за раз, не всё сразу:

```
internal/usecase/
  profile.go      — CreateMyProfile, UpdateMyProfile, SubmitForReview
  moderation.go   — ListForModeration, Moderate, ModerationHistory
  booking.go      — CreateBooking, GetBooking, ListBookings, CancelBooking,
                    ConfirmBookingByPayment, CancelBookingByPayment
  photo.go        — AddMasterPhoto, DeleteMasterPhoto, SetMasterCoverPhoto
  public.go       — GetPublicBySlug, ListPublic
```

Все пять structs разделяют одну зависимость `domain.MasterRepository`;
`NewMasterServer` (delivery/grpc) принимает их по отдельности или через
агрегирующий `MasterFacade` — в зависимости от того, сколько из них
нужно в одном gRPC-сервере.

**Ловушка при разбиении**  
`delivery/grpc/server.go` принимает сейчас `*usecase.MasterUseCase` конкретным
типом — потребует либо интерфейсов для каждого usecase, либо агрегатора.
Менять это надо одним PR вместе с разбиением, иначе сломаются сигнатуры.

**Triggers for prioritisation**  
- Появление нового домена (CRM-задачи, push-уведомления, аналитика мастера),
  который потребует отдельного usecase с другими зависимостями.
- Первый мёрж-конфликт в `master.go` при параллельной разработке.
- Файл перевалит за ~1500 строк.
