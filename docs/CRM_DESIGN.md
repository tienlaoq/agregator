# CRM — дизайн доработки

Статус: **черновик на согласование**. Код не пишем, пока не приняты решения из
раздела [Решения, которые нужно принять](#решения-которые-нужно-принять).

Документ описывает, как развить текущую CRM (`crm-service` + хендлеры в
api-gateway + фронт `/owner/venues/{venueId}/crm`) от «список задач + персонал»
до полноценной CRM с профилем гостя (Customer 360), не ломая архитектурные
инварианты проекта (Clean Architecture, gRPC sync, NATS async, БД на сервис).

---

## 1. Что есть сейчас (база отсчёта)

| Блок | Где | Чем ограничен |
|---|---|---|
| RBAC/команда | `crm-service` → `venue_staff` | `owner` (неявно через `venues.owner_id`), `manager`, `staff`; добавляет/удаляет только владелец |
| Задачи | `crm-service` → `venue_crm_tasks` | write-once: статус только `open→done`, нет edit/reopen/reassign/delete, нет `due_at`/`priority`, нет `completed_by/at` |
| Заметки по броням | `booking-service` → `BookingStaffNote` | живут в другом сервисе; не связаны с задачами и гостем |

**Главный пробел:** сущности «клиент/гость» не существует. Брони привязаны к
`user_id`, но агрегации по гостю нет — владелец не знает своих постоянных
гостей, LTV, отток. `crm.proto` это и фиксирует: *«Future scope: leads, deals,
pipelines, customer communications log»*.

**Известные нюансы в коде, которые учитываем:**

- `manager` и `staff` сейчас **идентичны по правам** — `ensureMember`
  (`usecase/crm.go:43`) проверяет лишь «член/не член». Различие ролей —
  декоративное.
- Событие `booking.*` несёт минимум: `{booking_id, user_id, venue_id, status}`
  (`booking-service/internal/domain/outbox.go`).
- Доставка событий **at-least-once** (transactional outbox, `009_outbox_events`).
  Консьюмеры обязаны быть идемпотентны.
- `analytics-service` потребляет только `analytics.web`; per-guest агрегатов не
  ведёт → проекция гостя не дублирует аналитику.

---

## 2. Целевая архитектура

`crm-service` сегодня чисто синхронный (gRPC, делит БД с venue). Предложение:
сделать его **ещё и консьюмером событий** и построить read-модель гостя как
проекцию. Это опирается на outbox, который уже доделывается в `booking-service`.

```
booking-service ─┐
payment-service ─┼─ outbox → NATS JetStream ─→ crm-service (НОВЫЙ consumer)
review-service  ─┘                                   │
                                                     ▼
                                        crm_guest_booking_facts  (идемпотентный леджер)
                                                     │  (re-aggregate по venue+user)
                                                     ▼
                                        crm_guest_profiles  (визиты, LTV, no-show)
                                                     │
                                          ListGuests / GetGuest (gRPC) → кабинет (Customer 360)
```

Паттерн консьюмера копируем с `analytics-service/internal/events/subscriber.go`
(durable push-consumer + `metrics.ObserveNATS`) и `review-service` (идемпотентность
по `booking_id`). DLQ — как `PAYMENTS_DLQ` / `events.DLQSubjectWildcard`.

### Идемпотентность проекции (важно при at-least-once)

Проекция строится как **детерминированная функция от фактов**, а не инкрементом
счётчиков (инкремент + повторная доставка = двойной учёт):

1. На каждое событие — **upsert** одной строки в `crm_guest_booking_facts`
   (ключ `booking_id`). Повторная доставка перезаписывает ту же строку → no-op.
2. После upsert — **пересчёт** агрегата `crm_guest_profiles` для затронутой
   пары `(venue_id, user_id)` из фактов (`SUM/COUNT`). O(броней гостя) — дёшево.

Защита от out-of-order/регресса статуса: у статусов известный порядок
(`pending < payment_pending < confirmed < completed`, плюс терминальный
`cancelled`). В факт пишем «самый продвинутый» виденный статус (по рангу), а не
просто последний.

---

## 3. Фаза A (Tier 0) — починить задачи и роли

Без событий, только `crm-service` + gateway + фронт. Это фундамент и быстрый
видимый результат.

### 3.1 Миграция `003_crm_tasks_lifecycle.up.sql`

```sql
ALTER TABLE venue_crm_tasks
    ADD COLUMN due_at       TIMESTAMPTZ,
    ADD COLUMN priority     TEXT NOT NULL DEFAULT 'normal'
                            CHECK (priority IN ('low', 'normal', 'high')),
    ADD COLUMN completed_by UUID,
    ADD COLUMN completed_at TIMESTAMPTZ;

-- статус: добавляем 'cancelled' (мягкое удаление задачи)
ALTER TABLE venue_crm_tasks DROP CONSTRAINT venue_crm_tasks_status_check;
ALTER TABLE venue_crm_tasks
    ADD CONSTRAINT venue_crm_tasks_status_check
    CHECK (status IN ('open', 'done', 'cancelled'));

-- индекс под «мои просроченные/срочные открытые задачи»
CREATE INDEX idx_venue_crm_tasks_assignee_open
    ON venue_crm_tasks (assignee_user_id, due_at)
    WHERE status = 'open';
```

### 3.2 Proto (`crm.proto`)

```proto
// Task — добавить поля:
//   optional google.protobuf.Timestamp due_at  = 11;
//   string priority                             = 12;  // low|normal|high
//   optional string completed_by                = 13;
//   optional google.protobuf.Timestamp completed_at = 14;

rpc UpdateTask(UpdateTaskRequest) returns (UpdateTaskResponse);  // title/body/due_at/priority/assignee
rpc ReopenTask(ReopenTaskRequest) returns (ReopenTaskResponse);  // done → open
rpc CancelTask(CancelTaskRequest) returns (CancelTaskResponse);  // → cancelled (мягкое удаление)
// CompleteTask: писать completed_by = actor_id, completed_at = now()
```

### 3.3 RBAC — реальная матрица прав

Заменить бинарный `ensureMember` на capability-проверку `can(access, action)`:

| Действие | owner | manager | staff |
|---|:---:|:---:|:---:|
| Видеть задачи/брони/заметки | ✅ | ✅ | ✅ |
| Создавать/закрывать **свои** задачи | ✅ | ✅ | ✅ |
| Редактировать/отменять **любые** задачи | ✅ | ✅ | — |
| Видеть финансы/LTV гостя | ✅ | ✅ | — |
| Управлять персоналом | ✅ | — | — |
| Карточка заведения | ✅ | — | — |

> Граница «менеджер приглашает персонал?» — **решение №5**.

### 3.4 Уведомления

При `assignee` ≠ автор — bell-уведомление исполнителю. Паттерн готов:
`staffInviteNotifier` / `NotifyStaffInvited` (`handler/notifications.go:291`),
добавить `NotifyTaskAssigned(ctx, assigneeID, venueID, taskID, title)`.

---

## 4. Фаза B (Tier 1) — профиль гостя (Customer 360)

### 4.1 Обогащение события брони (аддитивно, обратносовместимо)

`bookingEventPayload` (`domain/outbox.go`) — добавить поля. Имена существующих
**не трогаем** (downstream зависит), только добавляем:

```go
type bookingEventPayload struct {
    BookingID  string `json:"booking_id"`
    UserID     string `json:"user_id"`
    VenueID    string `json:"venue_id"`
    Status     string `json:"status"`
    TotalPrice int64  `json:"total_price"` // NEW — minor units, для LTV
    Date       string `json:"date"`        // NEW — YYYY-MM-DD, дата визита
    Guests     int32  `json:"guests"`      // NEW — размер компании
}
```

Источник правды один — `NewBookingEvent`. Старые консьюмеры (review,
notification) лишние поля игнорируют.

### 4.2 Миграции CRM

```sql
-- 004_crm_guest_booking_facts.up.sql — идемпотентный леджер (1 строка на бронь)
CREATE TABLE crm_guest_booking_facts (
    booking_id   UUID PRIMARY KEY,
    venue_id     UUID NOT NULL,
    user_id      UUID NOT NULL,
    status       TEXT NOT NULL,
    status_rank  SMALLINT NOT NULL,        -- для защиты от out-of-order
    total_price  BIGINT NOT NULL DEFAULT 0,
    visit_date   DATE,
    guests       INT NOT NULL DEFAULT 0,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_crm_facts_guest ON crm_guest_booking_facts (venue_id, user_id);

-- 005_crm_guest_profiles.up.sql — агрегат (проекция), grain (venue_id, user_id)
CREATE TABLE crm_guest_profiles (
    venue_id            UUID NOT NULL,
    user_id             UUID NOT NULL,
    first_visit_at      DATE,
    last_visit_at       DATE,
    last_booking_at     TIMESTAMPTZ,
    bookings_count      INT NOT NULL DEFAULT 0,   -- созданных
    visits_count        INT NOT NULL DEFAULT 0,   -- completed
    cancellations_count INT NOT NULL DEFAULT 0,
    no_show_count       INT NOT NULL DEFAULT 0,   -- см. решение №1
    total_spent         BIGINT NOT NULL DEFAULT 0,-- LTV, minor units
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (venue_id, user_id)
);
-- индексы под сегменты/сортировки
CREATE INDEX idx_crm_profiles_last_visit ON crm_guest_profiles (venue_id, last_visit_at);
CREATE INDEX idx_crm_profiles_ltv        ON crm_guest_profiles (venue_id, total_spent DESC);
```

> **PII (152-ФЗ):** имя/email/телефон гостя в CRM **не храним** — тянем из
> user-service на чтение, ровно как уже сделано для персонала
> (`venueStaffLoadUserDisplays`, `handler/venue_crm.go:43`). Профиль хранит
> только UUID + поведенческие агрегаты. Контакты/маркетинговое согласие —
> **решение №2**.

### 4.3 Консьюмер: какие события → какой эффект на факт

| Событие | Эффект на `crm_guest_booking_facts` (затем пересчёт профиля) |
|---|---|
| `booking.created` | upsert: status, `last_booking_at` |
| `booking.confirmed` | upsert status (ранг растёт) |
| `booking.completed` | upsert status; `visit_date`, `total_price` → `visits_count`, `total_spent`, `first/last_visit_at` |
| `booking.cancelled` | upsert status → `cancellations_count` |
| `payment.succeeded` (позже) | сверка/уточнение выручки — **решение №3** (источник LTV) |

### 4.4 Proto — поверхность Customer 360

```proto
rpc ListGuests(ListGuestsRequest) returns (ListGuestsResponse);
// venue_id, actor_id, фильтры: segment?, tag?, sort (last_visit|ltv|visits), пагинация
rpc GetGuest(GetGuestRequest) returns (GetGuestResponse);
// профиль + теги + последние брони + заметки (полная карточка 360)

message GuestProfile {
  string user_id = 1;
  string venue_id = 2;
  int32  bookings_count = 3;
  int32  visits_count = 4;
  int32  cancellations_count = 5;
  int32  no_show_count = 6;
  int64  total_spent = 7;
  google.protobuf.Timestamp first_visit_at = 8;
  google.protobuf.Timestamp last_visit_at = 9;
  repeated string tags = 10;
  repeated string segments = 11;   // вычисляемые, не хранятся
}
```

Gateway обогащает имя/email из user-service (как для персонала) и **режет
финансовые поля для роли `staff`** (см. матрицу 3.3).

### 4.5 Фронт

- `/owner/venues/{venueId}/crm/guests` — список гостей: сортировка по
  LTV/визитам/давности, бейджи сегментов.
- `/owner/venues/{venueId}/crm/guests/{userId}` — карточка 360: агрегаты, теги,
  лента броней, заметки гостя, кнопка «написать» (переиспользует
  `BookingChatPanel`).

---

## 5. Фаза C (Tier 1, продолжение) — теги, заметки, сегменты, единая лента

- **Теги** `crm_guest_tags(venue_id, user_id, tag, created_by, created_at)` —
  ручные метки (VIP, проблемный, корпоратив).
- **Заметки гостя** `crm_guest_notes(...)` — на уровне гостя, в отличие от
  заметок по брони. **Тех-долг:** `BookingStaffNote` живёт в `booking-service`,
  задачи и заметки гостя — в `crm-service`. Цель — единая «лента взаимодействий»
  (брони + заметки + задачи + чат + отзывы) в одном API. Завести запись в
  `TECH_DEBT.md`.
- **Сегменты — вычисляемые**, без таблицы в v1 (хардкод правил):
  - `new` — `bookings_count <= 1`
  - `regular` — `visits_count >= 3`
  - `vip` — `total_spent >= порог` (порог — конфиг venue)
  - `at_risk` — `visits_count >= 2 AND last_visit_at < now() - 90d`
  - `problematic` — `no_show_count >= 2 OR cancellations_count высокий`
  - Кастомные определения сегментов — отдельная фича позже (**решение №6**).

---

## 6. Фазы D и E (эскиз, после Tier 1)

- **Фаза D (Tier 2) — лиды/воронка:** заявки на корпоративы/группы/обратный
  звонок из чат-виджета. Простой kanban `Новый → Связались → КП →
  Выиграл/Проиграл`. Технически — это «задача с этапом воронки + сумма +
  ожидаемая дата».
- **Фаза E (Tier 3) — удержание:** по `booking.completed` — авто follow-up и
  запрос отзыва (петля с `review-service`); win-back для сегмента `at_risk`;
  напоминания о ДР/годовщине. Реализуется как консьюмер + планировщик задач.

---

## 7. Решения, которые нужно принять

| № | Вопрос | Рекомендация |
|---|---|---|
| 1 | **No-show**: статуса нет в домене брони. Заводить новый статус `no_show` в booking-service или выводить (confirmed + время визита прошло + не completed)? | Вывод на стороне booking-service + новое событие `booking.no_show`; иначе CRM придётся знать расписание. **Зависимость на booking-service.** |
| 2 | **Контакты + маркетинговое согласие** (152-ФЗ): где хранить (user-service vs crm), как собирать согласие? | Контакты — в user-service (read-only для CRM). Флаг согласия — в user-service, CRM только читает. Не дублировать PII в CRM. |
| 3 | **Источник LTV**: `booking.completed.total_price` или `payment.succeeded`? | v1 — `booking.completed.total_price` (без cross-stream join). Сверку с платежами — фаза E. |
| 4 | **Грейн профиля**: per-venue или roll-up per-owner? | Хранить per-`(venue_id, user_id)` (совпадает с URL-пространством и моделью доступа). Сводка по владельцу — агрегат на чтении позже. |
| 5 | **Менеджер приглашает персонал?** | По умолчанию — нет (только owner). Если владельцы попросят — отдельная capability. |
| 6 | **Сегменты**: хардкод v1 или кастомные определения? | Хардкод 5 сегментов в v1; конструктор сегментов — отдельная фича. |

---

## 8. Порядок реализации и оценка

| Фаза | Содержание | Зависимости | Объём |
|---|---|---|---|
| **A** | Жизненный цикл задач, RBAC-матрица, уведомления исполнителю | нет | S (1–2 дня) |
| **B** | Обогащение события + консьюмер + проекция + ListGuests/GetGuest + фронт 360 | outbox (готов), решения №1–4 | M (неделя) |
| **C** | Теги, заметки гостя, сегменты, единая лента | B | S–M |
| **D** | Лиды/воронка | B | M |
| **E** | Удержание/автоматизация | B, C, review-service | M |

Рекомендуемый старт: **A** (чистка долга, низкий риск) → **B** (главная
ценность). C/D/E — по приоритету бизнеса.
