# Code Review: `services/api-gateway`

Высокоуровневое ревью по четырём осям: архитектура, безопасность, производительность, качество кода. Дата ревью: 2026-05-14.

Ниже — то, что **уже сделано хорошо**, и то, что стоит улучшить. Каждый пункт помечен приоритетом: **P0** (критично/безопасность), **P1** (стоит починить в ближайшем спринте), **P2** (рефакторинг/качество).

---

## 1. Сильные стороны (что уже сделано хорошо)

- **Архитектура и слои.** `cmd/main.go` — единая точка сборки зависимостей, далее handlers (`internal/handler`), middleware (`internal/middleware`), realtime (`internal/realtime/chatws`), telemetry, metrics, persistence (`supportstore`). Gateway честно играет роль фасада: бизнес-логика лежит в downstream-сервисах, а gateway занимается auth/marshalling/rate-limit/upload — это правильно.
- **Каталог ошибок.** `internal/apicatalog/` — генерируется из `errors.yaml`, даёт стабильные машинные коды (`GATEWAY.AUTH.UNAUTHORIZED` и т.д.) и единый маппинг `gRPC code → HTTP code` через `FromGRPC`. Это аккуратнее, чем россыпь `http.Error(...)`.
- **Безопасность сделана осознанно, с комментариями.** Видны P0-фиксы прошлых ревью с пояснениями: ws-ticket с привязкой к Origin, `INTERNAL_SERVICE_TOKEN` как fatal в проде, защита от path-traversal в `absPathFromPublicURL`, magic-bytes валидация загружаемых картинок, `MaxBytesReader`, `SecurityHeaders` middleware.
- **Observability.** OpenTelemetry (OTLP HTTP), zerolog со встраиванием trace_id и request_id, Prometheus с лейблами по chi-pattern (`route` берётся из `chi.RouteContext.RoutePattern()`, а не `r.URL.Path` — иначе кардинальность была бы катастрофой).
- **gRPC dial defaults в `pkg/grpcutil`.** Keepalive + retry policy (UNAVAILABLE → до 4 попыток) для всех клиентов через `InsecureDialOptions()` — это снимает много транзиентных проблем без кастомного кода в gateway.
- **Graceful shutdown.** `signal.Notify`, `srv.Shutdown` с 15-секундным таймаутом, отдельный shutdown для OTel.
- **WebSocket-надёжность.** `wsReadLimit=8KiB`, ping/pong с `wsPongWait=60s`, `done`-канал для остановки ping-горутины — комментарий явно объясняет, почему **не** использовать `r.Context().Done()` (window между выходом из read-loop и cancel у HTTP-сервера). Это редкий уровень аккуратности.
- **Rate-limit fan-out.** Redis-лимитер с Lua-скриптом (атомарный `INCR + PEXPIRE`), in-memory fallback для dev с фоновой prune-горутиной. Стратегия `fail-open` для `forgot-password` сконфигурирована флагом — хорошо.
- **CORS.** Никаких `*` с credentials. Allowlist берётся из `CORS_ALLOWED_ORIGINS` → `FRONTEND_URL` → dev-defaults. WS `CheckOrigin` отдельно мапит на тот же allowlist + явная "empty-Origin" политика для не-браузеров с понятным rationale.
- **Тестовое покрытие.** Видны unit-тесты на ключевые middleware (`auth_test`, `cors_origins_test`, `forgot_password_ratelimit_test`), на хендлеры (`auth`, `chat`, `chat_thread_resolver`, `master`, `support`, `venue`), на hub и prometheus. Это уже не "галочка-тест".
- **Multi-stage Dockerfile.** `CGO_ENABLED=0` + alpine + non-glibc-зависимый бинарь.

---

## 2. Архитектура и структура

### 2.1 `main.go` перегружен — ~470 строк инициализации (P1)

`cmd/main.go` делает всё: парсинг env, создание восьми gRPC-клиентов, опциональный pgxpool, опциональный Redis, опциональный NATS, инициализация всех handler'ов, регистрация ~80 роутов. При следующем добавлении сервиса (notifications? search?) файл превратится в нечитаемый.

**Что стоит сделать:**

- Выделить **сборку зависимостей** (`buildDeps(ctx, log, cfg) (*deps, func(), error)`) и **построение роутера** (`buildRouter(deps) *chi.Mux`). Это упростит как чтение, так и unit-тестирование роутера.
- Конфигурацию (env-vars) собрать в `type Config struct { ... }` и `LoadConfig() (Config, error)` с `Validate()`. Сейчас env читается россыпью из 30+ мест, нет единой точки для логирования "что прочитано" и нет валидации до старта (например, `BASE_URL` без схемы или с trailing slash сейчас не проверяется).
- Каждый блок `grpc.NewClient(...) + log.Fatal` повторяется 8 раз — есть смысл сделать хелпер `mustDial(addr string, name string, opts ...) *grpc.ClientConn`.

### 2.2 Дублирование v1/v2 чат-роутов (P2)

```go
protected.With(chatCabinet).Get("/chat/ws", chatHandler.WS)
protected.With(chatCabinet).Get("/v2/chat/ws", chatHandler.WS)
// и ещё пять пар идентичных мэппингов
```

Сейчас v1 и v2 ходят в один хендлер. Версионирование протекло в payload (`type` для v1, `event` для v2) — это работает, но если задача — постепенная миграция, стоит:

- Завести middleware `injectAPIVersion(v)` который кладёт версию в контекст, а сам handler пишет либо `type`, либо `event`, либо оба. Сейчас все ответы возвращают и v1, и v2 поля одновременно — это рабочее, но не "breaking contract" как заявлено в комментарии.
- Или сделать subrouter `chi.Route("/v2/chat", ...)` с одним `Use(VersionV2)`.

### 2.3 `internal/handler` — слишком много (P2)

26 .go-файлов в одном пакете, ~9000 LOC. Это рабочая модель, но handlers `venue.go` (869 строк), `master.go` (687), `chat.go` (688) — кандидаты на разделение по доменам:

- `internal/handler/venue/` — list/search/photos/halls/crm/staff
- `internal/handler/master/` — то же самое
- `internal/handler/chat/` — REST + WS + resolver + peer_display

Каждый файл уже логически выделен (`venue_photos.go`, `venue_crm.go`, `venue_hall_photos.go`), просто папка-к-папке.

### 2.4 Миграции и хранилище в gateway (P1, архитектурное)

`migrations/001_support_tickets.up.sql` и `internal/supportstore` означают, что у gateway есть **собственная БД** (схема `support_db`). Это явно ломает один из принципов микросервисов из CLAUDE.md ("PostgreSQL per service"). Gateway должен оставаться stateless.

Варианты:

- Вынести support-тикеты в новый микросервис (`services/support-service`) с gRPC. Это вписывается в текущую архитектуру.
- Или признать, что support-store **— часть** gateway по дизайну, и явно это задокументировать в `CLAUDE.md`/`README` (с обоснованием).

Текущий middle-ground "ticketStore опционален, если PG_* не задан — фичу выключаем" — рабочий, но скрывает архитектурный долг.

### 2.5 Тонкие/тривиальные handlers (P2)

`UserHandler.GetMe` — два десятка строк, чтобы прокинуть `userId` из контекста в gRPC. Та же история в `BookingHandler.Get`, `*.Cancel`, etc. Это **не** плохо (gateway = маршаллинг), просто стоит признать факт и при возможности генерировать такие thin-handler'ы из proto (grpc-gateway/connect-go), а руками писать только то, что делает что-то особенное (multipart, OAuth, WS).

---

## 3. Безопасность

### 3.1 Логи и логирование секретов (P0)

`PaymentHandler.Webhook`:

```go
slog.Info("payment webhook received",
    "content_length", len(body),
    "content_type", r.Header.Get("Content-Type"),
)
```

— ок, body не логируется. Хорошо.

Но в `AnalyticsHandler.CollectEvent` пишется `RawJSON("props", propsJSON)` — а контракт "no PII in props" держится клиентом, не валидатором. **Сервер не должен на это полагаться.** Стоит:

- Либо whitelist'ить ключи (`utm_*`, `page`, `referrer`, ...) и игнорировать остальные.
- Либо хотя бы пройти `propsJSON` regexp'ом на email/телефон/UUID-токены и реджектить (или маскировать).

### 3.2 Доверие к `X-Forwarded-For` (P0)

`middleware/forgot_password_ratelimit.go::clientIP` берёт **первый** элемент `X-Forwarded-For` без проверки, что запрос реально пришёл от доверенного прокси:

```go
if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
    if i := strings.IndexByte(xff, ','); i >= 0 {
        xff = strings.TrimSpace(xff[:i])
    }
    return xff
}
```

Если gateway смотрит наружу (а в `main.go` он явно слушает `:HTTP_PORT`), любой клиент может подставить `X-Forwarded-For: <чужой_ip>` и:

1. обойти rate-limit на `/auth/forgot-password` (просто меняя XFF на каждый запрос);
2. отравить логи (а через них — алерты безопасности).

**Фикс:**

- Использовать `chi/middleware.RealIP` **с** проверкой "идёт ли запрос от доверенной подсети" (env `TRUSTED_PROXY_CIDRS`).
- Или, если gateway всегда за nginx/cloudflare, доверять только `r.RemoteAddr` и игнорировать XFF.
- Или брать **последний** элемент XFF (его пишет ближайший прокси), а не первый.

### 3.3 gRPC без TLS — `InsecureDialOptions()` для всех сервисов (P1)

Все 8 gRPC-клиентов поднимаются с `grpc.WithTransportCredentials(insecure.NewCredentials())`. В пределах одного namespace в k8s это типично, но:

- Между нодами/инстансами без mTLS трафик гуляет в открытом виде (Authorization tokens, refresh tokens, payment webhook body).
- `INTERNAL_SERVICE_TOKEN` — единственный "auth gate" — это shared secret, который тоже летит в plaintext.

Что делать: ввести `pkg/grpcutil.TLSDialOptions()` и переключать по env (`GRPC_TLS=true`), а в проде — обязать. Хотя бы между внешними зонами доверия (например, payment-service за платёжным контуром).

### 3.4 OAuth-state cookie + redirect_uri (P1)

```go
http.SetCookie(w, &http.Cookie{
    Name: "oauth_state", Value: state, Path: "/", MaxAge: 600,
    HttpOnly: true, SameSite: http.SameSiteLaxMode,
})
```

Не выставлено `Secure: true` — в проде по HTTPS cookie уйдёт, но в HTTP это уязвимо. Сделайте `Secure` автоматически когда `BASE_URL` начинается с `https://` (или жёстко требуйте https в проде).

Кроме того, `BASE_URL` берётся из env без валидации — если оператор поставит `BASE_URL=http://malicious-attacker.com`, OAuth-callback пойдёт туда. Валидируйте: `url.Parse`, schema = https в проде, host из allowlist.

### 3.5 OAuth: VK PKCE — `code_challenge = state` (P0)

```go
authURL := fmt.Sprintf("...&code_challenge_method=plain&code_challenge=%s", state)
...
"code_verifier": {state}
```

PKCE с `method=plain` и `code_verifier == state` — это **отсутствие PKCE как класс**. Цель PKCE — независимый от state secret. Если state утечёт (например, через Referer на промежуточной странице), атакующий получит и `state`, и `code_verifier`. Нужно:

1. Сгенерировать отдельный `code_verifier` (43-128 случайных байт), сохранить вместе со state в cookie (или Redis по ключу `state`).
2. `code_challenge` = `BASE64URL(SHA256(verifier))`, `method=S256`.
3. На callback подставить `code_verifier` из cookie/Redis.

### 3.6 OAuth: `rand.Read` без проверки ошибки (P2)

```go
func generateState() string {
    b := make([]byte, 16)
    rand.Read(b)
    ...
}
```

`crypto/rand.Read` теоретически может вернуть ошибку. На практике на Linux никогда не возвращает, но привычка молча игнорировать ошибки crypto-функций плохая. Проверьте `err` и в случае ошибки отдайте 500.

### 3.7 Rate-limit — только на `/auth/forgot-password` (P1)

В целом по API rate-limit'а нет. `/auth/login`, `/auth/register`, `/analytics/events`, `/payments/webhook`, `/support/contact` — все без лимитов. На login это критично (brute-force), на `/analytics/events` и `/support/contact` — слегка менее, но открывает спам/DoS.

Заведите общий middleware (на базе уже существующего Redis-лимитера) с per-IP и per-user лимитами, и развесьте по чувствительным эндпоинтам.

### 3.8 Recoverer возвращает stacktrace? (P2)

`chimw.Recoverer` по умолчанию пишет stacktrace в `w` если установлен env-флаг dev — в проде должен быть выключен. Стоит явно поставить `chimw.SetEntropy` или подменить на свой Recoverer, который только логгирует через zerolog (с request_id и trace_id) и отдаёт catalog-ошибку `GATEWAY.UPSTREAM.INTERNAL`.

### 3.9 `r.Body.Close()` через `defer` после `readJSON` (P2)

```go
func readJSON(r *http.Request, v any) error {
    defer r.Body.Close()
    return json.NewDecoder(r.Body).Decode(v)
}
```

Это нормально, но в `PaymentHandler.Webhook` body читается через `io.ReadAll(r.Body)` **без** `MaxBytesReader`. Платёжная вебхука теоретически может прислать 100 МБ — gateway это пропустит. Поставьте `r.Body = http.MaxBytesReader(w, r.Body, 256<<10)` (256 KiB достаточно для YooKassa).

### 3.10 Path-traversal в `venuePhotoExt` (P2 — pre-emptive)

`ServeVenueUploads` уже защищает через `filepath.Abs` + `HasPrefix(cleanRoot)`. Хорошо. Но рекомендую закрыть статику отдельным CDN/nginx — gateway не должен раздавать пользовательские картинки в проде. Это и вопрос производительности (см. ниже), и поверхность атаки.

### 3.11 Авторизация по ролям — строки (P2)

`RequireRole("venue_owner", "master")` — магические строки, легко опечататься. Заведите `pkg/roles`:

```go
const (
    RoleUser       = "user"
    RoleVenueOwner = "venue_owner"
    RoleMaster     = "master"
    RoleAdmin      = "admin"
)
```

— и используйте константы. Дополнительно — линтер на "RequireRole с неконстантой".

---

## 4. Производительность

### 4.1 ValidateToken на каждом запросе (P0)

`middleware.Auth` делает синхронный `authClient.ValidateToken(...)` на **каждый** аутентифицированный HTTP-запрос. При высокой нагрузке это:

- Дополнительный round-trip в auth-service по gRPC.
- Auth-service либо дёргает Redis/PG, либо парсит JWT с проверкой подписи.

Если access-token — это JWT (стандартный паттерн), gateway должен:

1. Получить JWKS / public key один раз при старте (с авто-rotate).
2. Валидировать токен **локально** через `jwt.Parse` + `ParseECDSA/RSA`.
3. ValidateToken через gRPC использовать **только** для blacklist/revocation проверки (опциональной, с коротким Redis-кэшем, скажем, 30 секунд).

Сейчас auth-service — узкое место для всего gateway. Это лежит в auth-service, но дизайн навязывает gateway.

Если JWT не используется (opaque token + БД), то хотя бы кэшируйте результат `ValidateToken` в Redis на 30-60 секунд по хешу токена.

### 4.2 `peerDisplayNamesBatch` — N+1 в чате? (P1)

Из обзора видно, что `attachPeerDisplayToThreadJSON` и `peerDisplayNamesBatch` (файл `chat_peer_display.go`, 200+ строк) делают batch — это хорошо. Проверьте, что для `ListThreads` (до 100 thread'ов с двумя участниками каждый) gateway делает **один** `GetUsers` (или batch-API в user-service), а не 200 отдельных `GetUser`. Если batch-API в user-service ещё нет — это P1.

### 4.3 Compress(5) глобально (P2)

`chimw.Compress(5)` сжимает **все** ответы, включая `/api/v1/uploads/*` (картинки уже сжаты, gzip их = трата CPU). Стоит:

- Исключить `/api/v1/uploads` из compress (поднять статику отдельной chi-`Route` **до** `r.Use(Compress)`, или сделать кастомный compress, который смотрит Content-Type).
- Понизить уровень до 3 — для JSON 5 vs 3 даёт ~5% размера, но в 2-3 раза больше CPU.

### 4.4 Статика через gateway = плохой sizing (P1)

`ServeVenueUploads` + uploaded files на локальном диске = одна реплика gateway хранит уникальные файлы у себя. При scale-out:

- Файлы, загруженные на replica A, недоступны через replica B.
- Сейчас в репо есть MinIO (упоминается в CLAUDE.md) — выгружайте через s3-клиент в MinIO, отдавайте либо через CDN, либо presigned URL.

### 4.5 `http.Server` таймауты (P2)

```go
ReadTimeout:  90 * time.Second,
WriteTimeout: 90 * time.Second,
```

90 секунд для read+write — много для REST. Понимаю, что это сделано под WebSocket-апгрейды (где connection долго живёт), но это **снимается через `http.TimeoutHandler` на не-WS эндпоинтах** или подменой timeout'ов внутри handler'ов. Сейчас один медленный клиент может занять worker на 90 секунд × N запросов.

Альтернатива — `ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30s, IdleTimeout: 120s` + долгие операции (форгот-пароль с 55s) уже сами ставят `context.WithTimeout`.

### 4.6 In-memory chat limiter (P2)

`newInMemoryChatLimiter` + prune-горутина — это хороший fallback, но при scale-out gateway каждая реплика будет иметь свой счётчик (т.е. реальный лимит = `sendRateMax * replicas`). В проде гарантируйте Redis-лимитер (или сделайте startup-check: `if env=production && redisLimiter==nil { log.Fatal }`).

### 4.7 `json.NewEncoder(w).Encode(v)` без буферизации (P2)

`writeJSON` пишет напрямую в `http.ResponseWriter`. Если в середине encode упадёт ошибка маршалинга (NaN, циклическая ссылка), клиент получит частичный JSON + 200 OK. Безопаснее:

```go
b, err := json.Marshal(v)
if err != nil { ... }
w.Header().Set("Content-Length", strconv.Itoa(len(b)))
w.WriteHeader(code)
_, _ = w.Write(b)
```

Минусом потеря streaming, но для JSON-ответов gateway это норма.

### 4.8 gRPC retry на write-методах (P1)

`pkg/grpcutil.ClientDialOptions()` ставит retry на **все** методы (`"name": [{}]`) для `UNAVAILABLE`. Это безопасно для read'ов, но для **не-идемпотентных write'ов** (CreateBooking, CreateReview, SendMessage) retry на UNAVAILABLE может привести к дубликатам, если первая попытка успела дойти до сервера и вернула ошибку **после** записи.

Что делать:

- Либо ограничить retry только методами `Get*`/`List*` (через `methodConfig.name` массив).
- Либо сделать всю запись идемпотентной (на уровне gRPC handler'ов — `ClientMsgId` для SendMessage уже сделан, для Create — нет).

---

## 5. Качество кода и best practices

### 5.1 `strconv.Atoi(r.URL.Query().Get("page"))` без обработки ошибки (P2)

Паттерн `page, _ := strconv.Atoi(...)` повторяется в venue/booking/master десятки раз. При невалидном вводе (`?page=abc`) получается `page=0`, и downstream должен сам это разруливать. Это:

- Скрывает баги клиентов (они не видят 400).
- Делает API "толерантным", что потом усложняет рефакторинг (нельзя ужесточить).

Заведите helper `parseIntDefault(r, key, def, min, max int) (int, error)` и возвращайте `apicatalog.GatewayRequestInvalidQuery`, если значение есть, но невалидно.

### 5.2 `_ = c.SendJSON(...)` — игнор ошибок WS (P2)

В `chat.go` 10+ мест с `_ = c.SendJSON(...)`. Если ошибка означает "connection closed", это ок (она пробрасывается в writeLoop и закрывает клиента). Но для read-loop иногда полезно различать "клиент отвалился" и "у нас баг", чтобы не молчать в проде. Минимум — `_ = c.SendJSON(...)` пометьте `// best-effort: writeLoop closes on real errors`, чтобы было понятно, что игнор намеренный.

### 5.3 zerolog vs slog (P2)

В коде смешиваются `zerolog` (через `pkg/logger`) и `log/slog` (в `chat.go`, `payment.go`, `analytics.go` через global default). Это создаёт две системы логирования с разными форматами. Выберите одну (zerolog уже инжектится в middleware), и второй замените.

### 5.4 Context keys через `type ctxKey string` (P2)

```go
type ctxKey string
const CtxUserID ctxKey = "user_id"
```

Безопаснее, чем raw string, но всё ещё может коллидировать с другими пакетами, использующими `type ctxKey string`. Идиоматичный Go:

```go
type userIDKey struct{}
var ctxUserID = userIDKey{}
```

— ключ-структура с приватным типом, коллизия невозможна.

### 5.5 Магические числа (P2)

```go
sendRateMax = 20
wsTicketTTL = 90 * time.Second
maxVenuePhotoBytes = 5 << 20
supportMaxMessageLen = 4000
```

— рассыпаны по handler'ам. Окей для констант пакета, но это **бизнес-ограничения**, которые должны быть в одном месте (или хотя бы в `internal/limits/limits.go`) + опционально оверрайдиться env-vars для нагрузочного тестирования.

### 5.6 Test helpers и таблицы (P2)

Глядя на список тестов — есть много `*_test.go` с разумным покрытием. Стоит проверить:

- Есть ли integration-тест на полный roundtrip "JWT → middleware → handler → mock gRPC" хотя бы для одного flow?
- В тестах middleware/handler'ов — используется ли httptest.NewServer или ручной chi.Mux? Прогон через chi важен, потому что multipart, URLParam, RoutePattern работают только через router.

### 5.7 `redirectError` отдаёт сообщения в URL (P2)

```go
errURL := fmt.Sprintf("%s/auth/login?error=%s", h.frontURL, url.QueryEscape(msg))
```

Тексты ошибок ("failed to exchange code", "email not provided by google") уходят в URL клиента. Эту строку не стоит делать internal-debug-сообщением. Заведите enum-коды (`oauth_state_mismatch`, `oauth_exchange_failed`, `oauth_no_email`), фронтенд их сам локализует.

### 5.8 `fmt.Sprintf` для строящихся URL (P2)

В `oauthLogin` callback URL строится через `Sprintf` с `url.QueryEscape`. Лучше через `url.Values.Encode()`:

```go
u, _ := url.Parse(h.frontURL + "/auth/callback")
q := u.Query()
q.Set("access_token", resp.AccessToken)
q.Set("refresh_token", resp.RefreshToken)
q.Set("user_id", resp.UserId)
u.RawQuery = q.Encode()
http.Redirect(w, r, u.String(), ...)
```

— устойчивее к багам с экранированием.

### 5.9 Передача access/refresh-токенов через query-параметр (P0)

```go
callbackURL := fmt.Sprintf("%s/auth/callback?access_token=%s&refresh_token=%s&user_id=%s", ...)
http.Redirect(w, r, callbackURL, http.StatusTemporaryRedirect)
```

Токены в **URL** = токены в:

- Логах прокси/CDN.
- `Referer` заголовке (если на `/auth/callback` есть external image/script).
- Истории браузера.

Это **серьёзная уязвимость**. Стандартный паттерн:

- Сетим refresh_token как `HttpOnly; Secure; SameSite=Lax` cookie.
- access_token в URL не передаём; вместо этого делаем редирект на `/auth/callback` без параметров, фронтенд делает POST `/auth/exchange` с CSRF-токеном, получает access_token в body, кладёт в memory.

Если архитектура SPA без BFF — хотя бы передавайте через **fragment** (`#access_token=...`) — это не уходит в Referer/логи (но всё ещё в истории, поэтому лучше — POST через intermediate-форму).

---

## 6. Сводная таблица приоритетов

| Приоритет | Что | Где |
|---|---|---|
| **P0** | OAuth: токены в query callback URL | `internal/handler/oauth.go::oauthLogin` |
| **P0** | VK PKCE: `code_challenge = state` (PKCE отсутствует) | `oauth.go::VKRedirect/exchangeVKCode` |
| **P0** | `X-Forwarded-For` без проверки доверенного прокси | `middleware/forgot_password_ratelimit.go::clientIP` |
| **P0** | Analytics: PII через клиентский контракт | `handler/analytics.go::CollectEvent` |
| **P0** | `ValidateToken` на каждый запрос — bottleneck | `middleware/auth.go::Auth` |
| **P1** | gRPC без TLS (`InsecureDialOptions` везде) | `cmd/main.go` |
| **P1** | OAuth cookie без `Secure`, `BASE_URL` без валидации | `oauth.go` |
| **P1** | Rate-limit только на forgot-password | `cmd/main.go` |
| **P1** | gRPC retry на не-идемпотентных write'ах | `pkg/grpcutil/dial.go` (поведение протекает в gateway) |
| **P1** | Статика и uploads через gateway вместо MinIO/CDN | `venue_photos.go`, `master_photos.go` |
| **P1** | `main.go` перегружен (~470 LOC) | `cmd/main.go` |
| **P1** | Support-store с собственной БД в gateway | `internal/supportstore/`, `migrations/` |
| **P1** | `peerDisplayNamesBatch` — убедиться, что N+1 нет | `chat_peer_display.go` |
| **P2** | Дубль v1/v2 чат-роутов | `cmd/main.go` |
| **P2** | Recoverer без custom logger'а | `cmd/main.go` |
| **P2** | `Compress(5)` на uploads | `cmd/main.go` |
| **P2** | Server timeouts 90s/90s — слишком долго | `cmd/main.go` |
| **P2** | `strconv.Atoi(...)+_` без 400 | `venue.go`, `booking.go`, `master.go` |
| **P2** | zerolog vs slog mix | `chat.go`, `payment.go`, `analytics.go` |
| **P2** | Магические числа без env-override | весь handler |
| **P2** | `rand.Read` без error check | `oauth.go::generateState` |
| **P2** | Payment webhook без `MaxBytesReader` | `handler/payment.go` |
| **P2** | Роли как магические строки | весь handler |
| **P2** | `internal/handler` — 9k LOC в одном пакете | `internal/handler/` |
| **P2** | `writeJSON` без буферизации | `handler/response.go` |
| **P2** | In-memory chat limiter в проде | `chat.go::newInMemoryChatLimiter` |
| **P2** | Тонкие thin-handlers можно генерировать | весь handler |

---

## 7. Что я бы сделал в первую очередь

Если из всего списка нужно выбрать **5 фиксов на эту неделю**:

1. **Убрать access/refresh-токены из OAuth callback URL** (P0, ~полдня работы).
2. **Локальная валидация JWT в `middleware.Auth`** (P0, день работы; даст огромный буст по latency).
3. **Доверенный X-Forwarded-For** (P0, час работы — env `TRUSTED_PROXY_CIDRS` + `chi/middleware.RealIP`).
4. **Per-IP rate-limit на `/auth/login` и `/auth/register`** (P1, день — переиспользуется `RedisLimiter`).
5. **Корректный PKCE для VK OAuth** (P0, полдня).

Остальные P1 — в бэклог на следующий спринт.

---

## 8. Что **точно делать не нужно** (anti-pattern)

- Не вводите серверный кэш JWT в Redis "вместо" локальной валидации. Локальная валидация JWT через JWKS — стандартный путь.
- Не выкидывайте chi ради чего-то "более модного" (echo, gin). Текущий chi-роутер аккуратно использует subrouters и middleware-chains.
- Не уходите с zerolog на slog "ради стандартной библиотеки". У zerolog лучше perf на горячем пути, переход даст vague benefit и большой diff.
- Не выносите apicatalog в shared pkg — он специфичен gateway. Если другие сервисы захотят коды ошибок, у них будет **свой** catalog.

---

*Ревью покрывает только `services/api-gateway`. Если интересно — следующим шагом можно глубже копнуть в `internal/handler/chat.go` (WS-протокол) или сделать аналогичное ревью одного из downstream-сервисов.*
