# Code Review v2: `services/api-gateway`

Свежее независимое ревью. Дата: 2026-05-14.

Это второе ревью — но я смотрю на код без оглядки на предыдущий отчёт: ищу проблемы заново. Структура отчёта та же: что хорошо, и что улучшить по 4 осям.

---

## 1. Общее впечатление

`services/api-gateway` производит впечатление зрелого продакшен-сервиса. Видны несколько ключевых отличий от типового стартап-кода:

- `cmd/` корректно разделён на `main.go` (только wiring) + `config.go` (env + Validate) + `deps.go` (открытие сокетов + cleanup) + `router.go` (chi-routes). Это эталонная организация Go-сервиса.
- Лимиты вынесены в отдельный пакет `internal/limits/` с env-override через `GATEWAY_LIMIT_*` (~25 параметров) — идеально для нагрузочного тестирования.
- TLS для gRPC сделан через `pkg/grpcutil.DialOptions()` с fatal-падением в проде, если `GRPC_TLS≠true`.
- JWT валидируется локально (HMAC через `pkg/auth.ValidateAccessToken`), gRPC-вызов `auth.ValidateToken` на горячем пути убран.
- OAuth: токены не в URL (refresh → HttpOnly cookie с `banya_refresh`, access → fragment `#access_token=...`), PKCE S256 c независимым verifier.
- Загрузки картинок: абстракция `storage.Uploader` (DiskUploader для dev, MinioUploader для прод) — gateway больше не привязан к локальному диску.
- Per-endpoint rate-limit на login/register/analytics/support/webhook + RealIP middleware с allowlist по `TRUSTED_PROXY_CIDRS`.
- Recoverer заменён на свой, который пишет в zerolog с request_id/trace_id и отдаёт catalog-ошибку.
- `writeJSON` теперь делает marshal-then-write — нет частичных ответов с 200 OK.
- Роли вынесены в `pkg/roles` (Role-тип), `RequireRole` принимает только константы.
- `peerDisplayNamesBatch` использует batch-API `GetUsersBatch` — N+1 по пользователям закрыт.

Это очень качественный набор фиксов. Ниже — что ещё стоит починить.

---

## 2. Архитектура и структура

### 2.1 `internal/handler` всё ещё ~10k LOC в одном пакете (P2)

26+ файлов в `internal/handler`, среди них `venue.go` (888 строк), `master.go` (710), `chat.go` (691), `support.go` (516), `oauth.go` (524). Это рабочее, но не идеальное состояние. Если каталог продолжит расти (а он растёт — новый thread_resolver, peer_display), то рано или поздно появится циклический импорт через `response.go` (риск отмечен в `docs/TECH_DEBT.md`, согласно CLAUDE.md).

Сейчас файлы группируются по префиксам (`venue_*`, `chat_*`, `master_*`). Натуральный следующий шаг — превратить префиксы в подпакеты. Конкретно:

```
internal/handler/
  ├── chat/ (chat.go, chat_test.go, peer_display.go, thread_resolver.go)
  ├── venue/ (venue.go, venue_crm.go, venue_photos.go, venue_hall_photos.go)
  ├── master/ (master.go, master_photos.go)
  ├── support/
  ├── booking/
  ├── auth/ (auth.go, auth_password.go, oauth.go)
  └── analytics.go, payment.go, user.go, review.go, response.go, health.go
```

С точки зрения wiring в `router.go` ничего не меняется (всё ещё `handler.NewVenueHandler(...)` через alias-импорты).

### 2.2 `chat.go` всё ещё гибрид WS + REST + helpers (P2)

`chat.go` (691 строка) делает:

- HTTP handlers (`EnsureThread`, `ListThreads`, ...).
- WS upgrade + read/write loops.
- In-memory rate limiter (`inMemoryChatLimiter`).
- JSON-сериализация `chatThreadToJSON` / `chatMessageToJSON`.
- NATS fan-out (`emitToUsers`).

Минимум — выделить `inMemoryChatLimiter` в `internal/ratelimit/inmemory.go` (он не специфичен для чата, и сейчас он дублирует `forgotPasswordHits` из `forgot_password_ratelimit.go` — оба делают sliding window in-memory). Это сейчас две независимых реализации одного и того же алгоритма.

### 2.3 `supportstore` в gateway = собственная БД (P1, не закрыто с v1)

`internal/supportstore` + миграции `migrations/001_support_tickets.up.sql` остались на месте. Gateway по-прежнему имеет собственный Postgres (`support_db`), что ломает один из принципов микросервисов из CLAUDE.md ("Postgres per service" — gateway не должен быть сервисом-хранилищем).

Это не P0, но архитектурный долг остаётся. В отчёте v1 предлагалось вынести в `services/support-service` — пока не сделано.

### 2.4 Dual v1/v2 чат-роутов отдают одинаковый JSON (P2, не закрыто)

В `router.go` v1 и v2 роуты по-прежнему мэппятся в один хендлер, который **всегда** пишет и `type:"message_new"`, и `event:"chat.message.created"`. Это значит, что "breaking contract" в коде не существует — v2-клиент видит и v1-поля, v1-клиент видит v2-поля. Что бы это значило в реальности — стоит явно решить:

1. Удалить v1 (если фронт уже мигрировал).
2. Или поставить middleware `injectAPIVersion(2)` и в handler условно писать только нужные поля.

Сейчас "версионирование" — псевдо, на уровне URL, без реальной семантики.

### 2.5 Маленький нит: `handler.NewPaymentHandler` принимает log, а `handler.NewBookingHandler` — нет (P2)

Несогласованный конструктор: некоторые handler'ы берут zerolog (chat, support, payment, oauth, analytics), некоторые — нет (auth, user, venue, booking, review, master). Для логирования внутри handler'а нужен логгер. Сейчас часть handler'ов скорее всего использует глобальный default logger из zerolog/slog, и в них пропадают request_id/trace_id.

Стандартизуйте: всегда передавать `zerolog.Logger` в конструктор. Это +1 параметр в 6 местах и решит проблему "тихих" логов.

---

## 3. Безопасность

### 3.1 JWT HS256 — симметричный секрет (P1)

`pkg/auth.ValidateAccessToken` использует HS256: один и тот же `JWT_SECRET` в auth-service (выпуск) и api-gateway (валидация). Это работающий вариант для микросервисов в одном trust-зоне, но:

- **Любой сервис, имеющий `JWT_SECRET`, может выпускать токены** (от имени любого user_id с любой ролью). Если gateway взломают, атакующий сразу получает full admin access без необходимости компрометировать auth-service.
- Утечка `JWT_SECRET` (через env-dump в log/error/CrashReport) скомпрометирует **всю** систему, а не только сервис, у которого она утекла.

**Идиоматический фикс:** ES256/RS256 с асимметричным ключом. auth-service хранит private key и подписывает; gateway получает public key (через JWKS endpoint или статичный файл) и только верифицирует. Это превращает gateway из "может всё" в "может только проверить".

Если ES256/RS256 — большой рефакторинг, минимум — заведите rotation policy для HS256 (две версии секрета, `JWT_SECRET_CURRENT` + `JWT_SECRET_PREV`, валидация против обоих).

### 3.2 JWT: header не проверяется (P1)

```go
parts := strings.Split(token, ".")
if len(parts) != 3 { return nil, ErrInvalidToken }
payload, err := base64.RawURLEncoding.DecodeString(parts[1])
...
sig := computeHMAC(secret, parts[0]+"."+parts[1])
if parts[2] != sig { return nil, ErrInvalidToken }
```

Header (`parts[0]`) не парсится и не проверяется на `alg=HS256`. Это значит, что если фронтенд (или кто-то ещё) пришлёт JWT с `alg=none`, то наш код:

1. Проверит подпись = HMAC-SHA256 над header.payload — она будет правильной, потому что HMAC вычисляется на нашей стороне с **нашим** секретом.
2. Декодирует payload.
3. Пройдёт.

Wait — на самом деле, в данном коде это безопасно, потому что мы всегда **сами** вычисляем HMAC и сверяем. Атакующий не может подобрать токен с другой подписью.

Но проблема в другом: если завтра вы захотите поддержать ES256 параллельно с HS256 (для миграции на асимметричный ключ), то надо будет читать `alg` из header. Сейчас этого нет, поэтому **есть риск "alg confusion attack"** при будущем рефакторинге — стандартная JWT-уязвимость. Лучше сразу:

```go
var hdr struct{ Alg string `json:"alg"` }
hdrBytes, _ := base64.RawURLEncoding.DecodeString(parts[0])
json.Unmarshal(hdrBytes, &hdr)
if hdr.Alg != "HS256" { return nil, ErrInvalidToken }
```

И использовать `hmac.Equal([]byte(parts[2]), []byte(sig))` вместо `==` — `==` для строк не constant-time (timing side-channel на подписи).

### 3.3 OAuth state cookie не привязан к state-значению (P2)

`oauth_state` — это одно cookie, общее для Google и VK. Если пользователь начнёт логин через Google, потом откроет другую вкладку и начнёт через VK, то Google-state перетрётся VK-state'ом, и в Google callback придёт mismatch (или, наоборот, VK-state совпадёт со state из URL Google, и пройдёт). Маловероятный сценарий, но симптом более глубокой проблемы: state хранится в global cookie, а не в session-scoped store.

Лучше: имя cookie с провайдером (`oauth_state_google` / `oauth_state_vk`) или хранение state в Redis по короткому random ID (cookie — только ID).

То же касается `vk_pkce_verifier` — он сейчас single-cookie, не shared между параллельными OAuth flow.

### 3.4 `setStateCookie` без SameSite=Strict (P2)

```go
SameSite: http.SameSiteLaxMode
```

Для OAuth state cookie `Lax` — корректный выбор (`Strict` сломает редирект с провайдера). Но для PKCE verifier, который не нужен браузеру в момент редиректа от провайдера на callback (он нужен только серверной части на callback'е), можно использовать `Strict`. Это не критично, но добавит +1 уровень защиты.

### 3.5 OAuth: `rand.Read` без проверки ошибки (P2, не закрыто с v1)

```go
func generateState() string {
    b := make([]byte, 16)
    rand.Read(b)  // ← err ignored
    ...
}

func generatePKCE() (verifier, challenge string) {
    b := make([]byte, 32)
    rand.Read(b)  // ← err ignored
    ...
}
```

На Linux никогда не падает на практике, но в Go стиль "молча игнорировать crypto-ошибки" не рекомендуется. Особенно в коде, который генерирует security-критичные значения.

### 3.6 `INTERNAL_SERVICE_TOKEN` всё ещё в plaintext при TLS=false (P1)

В `deps.go`:

```go
chatConn, err := mustDial(cfg.ChatAddr, "chat-service",
    grpc.WithUnaryInterceptor(grpcutil.ServiceTokenClientInterceptor(cfg.InternalServiceToken)),
)
```

Если `GRPC_TLS=false` (вне продакшена), service-token летит в plaintext. Это ожидаемо для dev, но стоит явно задокументировать в `CLAUDE.md` / TECH_DEBT, что **в любой среде, где есть network-sniff'еры (CI, shared dev cluster), `INTERNAL_SERVICE_TOKEN` должен быть либо пустым (отключающим service-auth), либо передаваться по TLS**.

Сейчас `Validate()` проверяет только наличие токена, не транспорта.

### 3.7 RealIP middleware: `r.RemoteAddr = ip + ":0"` (P2)

```go
if strings.ContainsAny(ip, ":") {
    r.RemoteAddr = "[" + ip + "]:0"
} else {
    r.RemoteAddr = ip + ":0"
}
```

Подмена реального порта на `:0` — это нормально, но порт `0` зарезервирован в TCP. Если какой-то middleware-логгер пишет порт в access-логе и потом эти логи парсятся (например, для определения NAT-сессии), он получит фиктивный `0`. Лучше использовать оригинальный порт или вообще исключить порт (но `net.SplitHostPort` ожидает порт). Минимум — добавить комментарий "the :0 is a placeholder; do not log it as a real client port".

Также: подмена `r.RemoteAddr` вместо использования context-key — это спорное решение. Если какой-то handler хочет узнать реальный peer (т.е. proxy IP, до RealIP-перезаписи), то возможности нет. Чище — оставить `r.RemoteAddr` нетронутым и положить real client IP в context:

```go
ctx := context.WithValue(r.Context(), ctxRealIP, ip)
```

— и обновить `clientIP(r)` чтобы он читал из context, а не из `RemoteAddr`. Это invasive change, но архитектурно правильнее.

### 3.8 `r.URL.Query().Get("access_token")` в `bearerTokenFromRequest` (P1)

```go
qToken := strings.TrimSpace(r.URL.Query().Get("access_token"))
if qToken != "" {
    return qToken, true, false
}
```

Это **публичный fallback** на query-параметр, не только для WS. Это значит, что любой GET-запрос с `?access_token=...` пройдёт авторизацию. И что:

- Access token уходит в access-логи nginx/CDN.
- Access token уходит в Referer (если внутри страницы есть external image/script).
- Access token уходит в browser history.

В коде комментарий говорит "Browser WebSocket cannot set custom Authorization headers" — что верно, но WS использует **другой** механизм (`ws_ticket` query parameter, который **одноразовый** и хранится в Redis).

Для обычных HTTP запросов `?access_token=` принимать **нельзя**. Если это поддерживается для каких-то специфических кейсов (legacy clients, server-side scrapers), хотя бы:

- Ограничьте только методом GET? — нет, ещё хуже, потому что GET кеширует.
- Ограничьте определёнными путями? — лучше.
- Заведите явное env-флаг `ALLOW_QUERY_AUTH_TOKEN=true` с дефолтом `false`, и логируйте warning при использовании.

**Идеально:** убрать query-fallback вообще, а для WS оставить только `ws_ticket`.

### 3.9 Admin endpoints без дополнительных мер (P2)

```go
r.With(admin).Post("/admin/venues/{id}/moderate", venueHandler.Moderate)
```

— admin endpoints защищены только `RequireRole("admin")`. Это нормально, но для high-stakes операций (moderate, reply на support, ban пользователя) обычно добавляют:

- **Re-authentication** для критичных действий (требовать пароль ещё раз перед moderate).
- **Audit log** в неизменяемом хранилище (current/admin moderation history есть для masters, но не для venues и support replies).
- **2FA** для admin-роли.

Это не P0 для MVP, но стоит держать в roadmap.

### 3.10 Rate-limit для admin endpoints отсутствует (P2)

```go
r.With(admin).Post("/admin/support/reply", supportHandler.AdminReply)
```

— админ может через скрипт скомпрометированной учётки отправить миллион писем (`/admin/support/reply` шлёт SMTP). Стоит добавить `supportRL` или отдельный admin-RL и сюда.

### 3.11 Body limit не везде (P2)

`MaxBytesReader` применяется в:
- Payment webhook: 256 KiB ✓
- Venue/master photos: 5 MiB ✓
- Analytics: `io.LimitReader(r.Body, 8192)` ✓
- Support: 8192 ✓

Не применяется в:
- `/auth/register`, `/auth/login`, `/auth/forgot-password`, `/auth/reset-password` — все используют `readJSON`, который только декодит JSON. Атакующий может прислать `{"email": "<10MB строка>"}` и gateway пожрёт память до OOM.
- `/users/me` PATCH, `/bookings`, `/reviews`, `/venues` create — то же.

Заведите общий middleware `BodyLimit(maxBytes int64)` или модифицируйте `readJSON` чтобы он **всегда** обёртывал `r.Body` в `MaxBytesReader(64KiB)` по умолчанию. Это копеечное изменение с большим эффектом против spray-атак.

### 3.12 SMTP-rate-limit на admin reply (P2)

`/admin/support/reply` шлёт plain-text email. На каждый запрос — один SMTP-вызов. Без rate-limit'а скомпрометированный admin может отправить тысячи писем (типичная атака — заставить gateway отправить email от вашего домена на список получателей, ваш сервер попадёт в blocklist).

---

## 4. Производительность

### 4.1 `VenueHandler.AvailabilityBySlug` — N+1 (P0)

```go
slots := bookingCandidateStartTimes()  // 25 слотов: 10:00..22:00 шаг 30
for _, start := range slots {
    resp, err := h.client.CheckSlotAvailability(ctx, ...)
    ...
}
```

На каждый запрос `/venues/{slug}/availability` gateway делает **25 синхронных gRPC-вызовов** в venue-service. Это:

- 25× latency RTT, 25× pgx-query, 25× allocations.
- Если бронирование на 2 часа и пользователь смотрит availability за неделю, фронтенд может делать 7 таких запросов → **175 gRPC-вызовов на одного пользователя на загрузку страницы**.

**Фикс на стороне venue-service:** добавить `CheckSlotAvailabilityBatch` или `ListAvailableSlots`, который возвращает массив доступных слотов одним запросом. На gateway переписать handler на один вызов.

Это, пожалуй, самая большая производительная проблема, которую я вижу.

### 4.2 Batch GetUser есть, но `GetBooking`/`GetVenue` всё ещё N+1 (P1)

В `chat_peer_display.go`:

```go
for _, ref := range venueRefsUnique {
    b, err := h.booking.GetBooking(ctx, &bookingv1.GetBookingRequest{Id: ref})
    ...
}

for vid := range venueIDsNeedingName {
    v, err := h.venue.GetVenue(ctx, &venuev1.GetVenueRequest{Id: vid})
    ...
}

for _, ref := range masterRefsUnique {
    mb, err := h.master.GetMasterBooking(ctx, ...)
    ...
}
```

`GetUsersBatch` уже есть и используется. По аналогии нужны `GetBookingsBatch`, `GetVenuesBatch`, `GetMasterBookingsBatch`. На `ListThreads` с 100 thread'ами это сейчас даёт ~200-300 gRPC round-trip'ов.

Стоит вынести в shared `internal/handler/chat_peer_display.go` TODO-комментарий или TECH_DEBT-запись.

### 4.3 NATS retry на support webhook без backoff (P2)

```go
lastErr := h.postWebhook(body)
if lastErr != nil {
    lastErr = h.postWebhook(body)
}
```

Single retry без задержки. Если helpdesk упал на 30 секунд (deploy), оба вызова уйдут с интервалом ~5ms и оба упадут. Стоит:

```go
backoff := 500 * time.Millisecond
for attempt := 1; attempt <= 3 && lastErr != nil; attempt++ {
    time.Sleep(backoff * time.Duration(attempt))
    lastErr = h.postWebhook(body)
}
```

Или использовать `cenkalti/backoff` (уже в indirect deps через otlp).

### 4.4 `ChatWSPingInterval` (30s) > 0.5 × `ChatWSPongWait` (60s) (P2 — edge case)

```go
ChatWSPingInterval = 30 * time.Second
ChatWSPongWait     = 60 * time.Second
```

Если первый pong потеряется (TCP retransmit, GC pause на клиенте), следующий ping уйдёт через 30s, и при счастливом RTT клиент успеет ответить до истечения 60s. Но если GC-пауза на клиенте >30s, или связь нестабильна, мы можем закрыть connection ложно-позитивно.

Идиоматично: `pongWait ≥ pingInterval × 2 + slack`. Сейчас `60s = 30s × 2`, без slack'а. Безопаснее `pongWait = 75s` или `pingInterval = 25s`.

### 4.5 `ParseMultipartForm` без `r.MultipartReader` (P2)

```go
r.Body = http.MaxBytesReader(w, r.Body, maxVenuePhotoBytes+1024)
if err := r.ParseMultipartForm(maxVenuePhotoBytes); err != nil {
    writeCatalog(w, apicatalog.GatewayRequestInvalidMultipart)
    return
}
file, _, err := r.FormFile("photo")
```

`ParseMultipartForm(maxMemory)` буферизует в RAM до maxMemory, **остальное** пишет в temp-файлы на диске. С `maxMemory = 5 MiB` это вроде ок, но при 100 конкурентных аплоадах = 500 MiB RAM + созданные tmp-файлы (которые потом надо чистить).

Стримовый аналог:

```go
mr, err := r.MultipartReader()
for {
    part, err := mr.NextPart()
    if err == io.EOF { break }
    if part.FormName() == "photo" { ... stream to MinIO ... }
}
```

— не буферизует ничего, передаёт прямо в `h.storage.Put`. Для аплоадов фоточек это важно при высокой нагрузке.

### 4.6 NATS connection pool — single connection (P2)

```go
nc, err := nats.Connect(cfg.NATSUrl)
```

Все publishes идут через одну connection. При высоких pps на `chat.fanout` это может быть bottleneck (под нагрузкой 10k msg/s на одну connection — типично, под 50k+ начинаются проблемы). Стоит мерить и при необходимости использовать pool или JetStream.

### 4.7 retries оставлены для `ValidateToken` в gRPC (P2 — заметка)

В `dial.go`:

```go
{"method": "ValidateToken"},
```

— retry до 4 раз на UNAVAILABLE. Но `ValidateToken` теперь не вызывается на горячем пути (JWT валидируется локально). Метод остался для каких-то специфических сценариев (ws ticket issue?). Если он реально нигде не вызывается — уберите его из retry-config (это не влияет ни на что, чисто гигиена).

### 4.8 `http.DefaultClient.Do` в OAuth (P2)

```go
resp, err := http.DefaultClient.Do(req)
```

В `exchangeVKCode` и `fetchVKIDUserInfo`. `http.DefaultClient` не имеет таймаута, опирается только на `ctx`. Поскольку ctx уже с таймаутом 10s в callback'е — ок. Но `http.DefaultClient` глобальный — если кто-то ещё в gateway его модифицирует (transport, etc.), будут side effects.

Заведите `var vkHTTPClient = &http.Client{Timeout: 10 * time.Second}` на уровне пакета.

---

## 5. Качество кода и best practices

### 5.1 Дублирование sliding-window лимитера (P2)

`forgotPasswordHits` (в `forgot_password_ratelimit.go`) и `inMemoryChatLimiter` (в `chat.go`) — это **одна и та же** реализация sliding-window in-memory лимитера. С разными именами и без prune'а в первом случае.

Вынесите в `internal/ratelimit/inmemory.go` как `NewInMemorySliding(ctx, pruneInterval) ratelimit.Limiter`. Это уберёт две параллельных реализации.

### 5.2 `queryInt` — отлично, но не используется (P2)

В `response.go` появилась хорошая helper `queryInt` с написанием 400 при невалидном вводе. Но в коде она используется **редко**. Поищите `strconv.Atoi(r.URL.Query().Get(`:

```
internal/handler/venue.go:    page, _ := strconv.Atoi(r.URL.Query().Get("page"))
internal/handler/venue.go:    pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
internal/handler/booking.go:  page, _ := strconv.Atoi(q.Get("page"))
... ~10 мест
```

Везде заменить на `queryInt`. Это снимет толерантность gateway к мусорному input.

### 5.3 Mixed logger: `h.log.Warn()` vs default zerolog (P2)

Стало лучше: chat, support, payment, oauth, analytics получают `log zerolog.Logger`. Но в `chat_thread_resolver.go` (не показан в этом ревью, но виден в импортах) — там логгер тоже передан? Стоит проверить. И venue/booking/master/review/user/auth handlers всё ещё могут использовать default logger, у которого нет request_id.

### 5.4 `crypto/subtle.ConstantTimeCompare` для service token (P2)

В `pkg/grpcutil/servicetoken.go` (предположительно) сверяется shared token. Стоит убедиться, что сравнение через `crypto/subtle.ConstantTimeCompare`, а не `==` — иначе timing side-channel. Не проверял, но обычно эта ошибка живёт в `==` сравнениях секретов.

### 5.5 `defer r.Body.Close()` после `readJSON` (P2)

```go
func readJSON(r *http.Request, v any) error {
    defer r.Body.Close()
    return json.NewDecoder(r.Body).Decode(v)
}
```

Это нормально, но не работает на multipart (там body закрывается отдельно). Хорошо бы добавить комментарий "this is for non-multipart JSON requests only".

### 5.6 `parsePositiveInt` vs `queryInt` — два почти одинаковых хелпера (P2)

`chat.go`:

```go
func parsePositiveInt(raw string, def, max int32) int32 {
    v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 32)
    if err != nil || v <= 0 { return def }
    if int32(v) > max { return max }
    return int32(v)
}
```

vs `response.go::queryInt` — почти то же самое, но возвращает int и пишет 400. Унифицируйте.

### 5.7 `chatThreadToJSON` нормализует UUID к lowercase — но не везде (P2)

В `chat.go` UUID-поля приводятся к lowercase. В `bookingToJSON`, `venueToJSON`, `reviewToJSON` — не уверен. Если контракт API "все UUID lowercase", сделайте это везде через единый helper `normalizeUUID(s)` или, лучше, доменный type:

```go
type UUID string
func (u UUID) MarshalJSON() ([]byte, error) {
    return json.Marshal(strings.ToLower(string(u)))
}
```

— и используйте этот тип во всех response struct'ах. Это убирает россыпь `strings.ToLower(...)` из handler-кода.

### 5.8 `clampString` — не виден в реализации (P2)

Используется в `support.go` (`req.Topic = clampString(req.Topic, supportMaxTopicLen)`). Поищите тест на edge case — что если строка состоит из multi-byte UTF-8 символов и clamp срезает посередине байтового представления (получается невалидный UTF-8)? Если `clampString` режет по байтам — это баг. Должен резать по rune'ам или `utf8.RuneStart`.

### 5.9 `serverTimeouts` — `WriteTimeout: 0` (P1)

```go
WriteTimeout: 0, // disabled — see comment above
```

Комментарий объясняет почему — но это означает, что если handler зависнет (взаимная блокировка goroutine'ы в чужой DB-транзакции, blocked sendMessage в hub'е), connection не закроется по WriteTimeout. Защита от такого только через `ReadTimeout` (30s) и `IdleTimeout` (120s) — но если goroutine уже **читает stream** (WS), `ReadTimeout` тоже не сработает после успешного апгрейда.

Рекомендация: использовать `http.TimeoutHandler` для REST-routes (group ниже Compress), оставив WS отдельно без таймаута. Сейчас единственная защита от висящих handler'ов — `IdleTimeout`, который срабатывает только если **ничего** не происходит.

### 5.10 `oauthLogin` использует `_` при `url.Parse` (P2)

```go
callbackU, _ := url.Parse(h.frontURL + "/auth/callback")
```

Комментарий говорит "url.Parse cannot fail here — h.frontURL was validated at construction." Это корректно для текущего кода, но при рефакторинге `frontURL` валидация может уйти, и `url.Parse` начнёт молча возвращать nil. Лучше:

```go
callbackU, err := url.Parse(h.frontURL + "/auth/callback")
if err != nil {
    h.log.Error().Err(err).Msg("BUG: frontURL not parseable post-construction")
    h.redirectError(w, r, oauthErrAuthFailed, err)
    return
}
```

— защита от багов будущего рефакторинга.

### 5.11 `pkg/auth.Claims` — `Exp` как int64 (P2)

```go
type Claims struct {
    UserID string `json:"sub"`
    Email  string `json:"email"`
    Role   string `json:"role"`
    Exp    int64  `json:"exp"`
    Iat    int64  `json:"iat"`
}
```

JWT-spec позволяет `nbf`, `iss`, `aud`. Сейчас не проверяется ни одно. Минимум — проверка `nbf` (`not before`) обычно бесплатная: `if claims.Nbf > 0 && time.Now().Unix() < claims.Nbf { return ErrInvalidToken }`.

Также — нет проверки `iat` на разумность (`iat` далеко в будущем = подозрительно).

### 5.12 Контекст `r.Context()` в long-running handlers (P2)

В чате:

```go
go func() {
    for {
        select {
        case <-ticker.C:
            _ = rawConn.WriteMessage(websocket.PingMessage, nil)
        case <-done:
            return
        }
    }
}()
```

`done` закрывается через `defer close(done)` в `WS`. Это работает. Но если бы пришлось добавить третий канал (например, "пользователя забанили — отключить"), потребовалось бы переделать. Стандартная альтернатива — создать `subCtx, cancel := context.WithCancel(r.Context())` локально, ходить по `subCtx.Done()`. Сейчас комментарий говорит, почему **не** используется r.Context — корректно. Просто на будущее: subCtx убирает need в отдельном `done`-канале.

---

## 6. Что нового стоит сделать в следующем спринте

Сравнивая с предыдущим ревью, P0 закрыты практически все, кроме одного нового:

| Приоритет | Что | Где |
|---|---|---|
| **P0** | `AvailabilityBySlug` — 25 синхронных gRPC-вызовов на 1 HTTP | `venue.go::AvailabilityBySlug` (нужен batch API в venue-service) |
| **P0** | Query-параметр `?access_token=` принимается на любых HTTP-эндпоинтах | `middleware/auth.go::bearerTokenFromRequest` |
| **P1** | JWT HS256 (симметричный) — gateway может выпускать токены | `pkg/auth/jwt.go` → перейти на ES256/RS256 |
| **P1** | JWT validate без проверки alg (защита от alg-confusion при будущем рефакторинге) | `pkg/auth/jwt.go::ValidateAccessToken` |
| **P1** | `WriteTimeout: 0` — нет защиты от висящих REST-handler'ов | `cmd/router.go::serverTimeouts` |
| **P1** | Body-limit отсутствует для JSON-эндпоинтов (`/auth/register`, etc.) | везде, где `readJSON` |
| **P1** | `GetBookingsBatch`/`GetVenuesBatch` — N+1 в `peerDisplayNamesBatch` | `chat_peer_display.go` |
| **P1** | `supportstore` в gateway = своя БД | `internal/supportstore/`, `migrations/` |
| **P2** | `internal/handler` ~10k LOC в одном пакете | `internal/handler/` |
| **P2** | Дублирование sliding-window лимитера | `chat.go::inMemoryChatLimiter` + `forgot_password_ratelimit.go::forgotPasswordHits` |
| **P2** | Dual v1/v2 чат-роутов отдают одинаковый JSON | `router.go` + `chat.go` |
| **P2** | `queryInt` есть, но `strconv.Atoi(..., _)` ещё в 10+ местах | `venue.go`, `booking.go`, `master.go` |
| **P2** | `crypto/rand.Read` без проверки ошибки | `oauth.go::generateState`, `generatePKCE` |
| **P2** | OAuth state cookie общий для Google и VK | `oauth.go::setStateCookie` |
| **P2** | RealIP middleware подменяет `RemoteAddr` вместо context-key | `middleware/realip.go` |
| **P2** | `parsePositiveInt` vs `queryInt` — два схожих хелпера | `chat.go`, `response.go` |
| **P2** | Multipart `ParseMultipartForm` буферизует — стрим через `MultipartReader` | `venue_photos.go`, `master_photos.go` |
| **P2** | Inconsistent constructor logger arg | `NewBookingHandler`, `NewVenueHandler`, etc. |
| **P2** | `http.DefaultClient` в OAuth VK requests | `oauth.go::exchangeVKCode`, `fetchVKIDUserInfo` |
| **P2** | NATS retry на support webhook без backoff | `support.go::Contact` |
| **P2** | `ChatWSPongWait` = `ChatWSPingInterval × 2` без slack | `limits/limits.go` |
| **P2** | Rate-limit на admin endpoints отсутствует | `router.go` (`/admin/*`) |
| **P2** | Admin endpoints: нет 2FA, нет re-auth, audit-log только для master moderation | `router.go` (`/admin/*`) |
| **P2** | `clampString` — потенциальная порча UTF-8 при clamp по байтам | `support.go` (проверить реализацию) |
| **P2** | JWT не проверяет `nbf` | `pkg/auth/jwt.go` |
| **P2** | `http.DefaultClient`, `WriteTimeout=0`, retry с timing-side-channel в `parts[2] != sig` | `pkg/auth/jwt.go` |

---

## 7. Прогресс с предыдущего ревью (что закрыто)

Если коротко — почти всё:

- ✅ OAuth: токены не в URL (refresh → HttpOnly cookie, access → fragment).
- ✅ VK PKCE S256 с независимым verifier.
- ✅ JWT-валидация локально (нет gRPC ValidateToken на горячем пути).
- ✅ TLS для gRPC (`pkg/grpcutil.DialOptions()`, fatal в production без `GRPC_TLS=true`).
- ✅ Trusted X-Forwarded-For с allowlist по CIDR.
- ✅ Per-endpoint rate-limit (login/register/analytics/support/webhook).
- ✅ Custom Recoverer с zerolog + catalog-ошибкой.
- ✅ Compress(3) — только на API routes, не на статику.
- ✅ `MaxBytesReader` на payment webhook.
- ✅ `writeJSON` с буферизацией.
- ✅ `pkg/roles` — type-safe роли.
- ✅ Analytics whitelist props (utm_* + фиксированный список).
- ✅ Server timeouts: `ReadHeaderTimeout: 10s`, `ReadTimeout: 30s`, `IdleTimeout: 120s` (только `WriteTimeout: 0` оставлен — см. P1 выше).
- ✅ `main.go` разделён на `main.go` + `config.go` + `deps.go` + `router.go`.
- ✅ `BASE_URL` валидируется при старте.
- ✅ Лимиты в `internal/limits/` с env-override.
- ✅ Storage abstraction (Disk/MinIO).
- ✅ OAuth error codes — opaque enum, не leak текстов.
- ✅ Secure cookie flag для OAuth state (автоматически по схеме BASE_URL).
- ✅ `GetUsersBatch` для chat peer display.

Это очень качественный фолоу-ап на первое ревью. Большой респект автору фиксов.

---

## 8. Топ-5 на следующий спринт

1. **Убрать `?access_token=` из publik HTTP path** (только `ws_ticket` для WS) — час работы. Это новый P0, который проявляется при свежем взгляде на код.
2. **`AvailabilityBySlug` → batch API в venue-service** — пару дней, требует изменения в venue-service. Это самая большая perf-проблема.
3. **JWT: проверка `alg` явно, `hmac.Equal` вместо `==`** — час работы. Превентивно закрывает alg-confusion.
4. **`MaxBytesReader` глобально в `readJSON`** — полчаса.
5. **Перейти с HS256 на ES256/RS256 для JWT** — день-два. Это большое концептуальное улучшение security posture.

Остальное — в бэклог.

---

## 9. Что не делать

- Не пытайтесь "оптимизировать" в `internal/handler` сразу. Сначала закройте 5 пунктов выше — они дают реальное value. Структурный рефакторинг папок (P2) — когда будет 40+ файлов и циклы импортов начнут реально мешать.
- Не убирайте dual v1/v2 чат-роутов "потому что криво" — сначала проверьте, что фронт уже на v2. Удалить v1 можно одним коммитом, восстановить — большой dependency-chain.
- Не вводите Redis-кэш JWT validation. Локальная валидация уже сделана идеально, кэш только усложнит.
- Не переделывайте `supportstore` под микросервис прямо сейчас — это сильно изменит API gateway, лучше включить в более крупный refactor когда (если) появится notifications-service.

---

*Если хочется ещё одного раунда — следующим шагом разумно глубоко посмотреть либо `chat.go` (там есть нюансы с consistency между REST и WS) либо сделать ревью одного из downstream-сервисов (auth-service выглядит самым критичным по security posture).*
