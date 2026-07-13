# Нагрузочное тестирование — k6

Сценарии нагрузки на api-gateway. Первый и основной — публичный каталожный
трафик (без авторизации), самый частый в проде (SEO/каталог).

## Что тестируется

`public-read.js` — модель «посетитель листает каталог»:

| Шаг | Эндпоинт | Примечание |
|---|---|---|
| Список заведений | `GET /api/v1/venues` | пагинация + сортировка |
| Поиск | `GET /api/v1/venues/search` | по городу/типу |
| Популярные города | `GET /api/v1/analytics/popular-cities` | лёгкая ручка главной |
| Карточка заведения | `GET /api/v1/venues/{slug}` | по реальному слагу из каталога |
| Отзывы | `GET /api/v1/venues/{venueId}/reviews` | |
| Список мастеров | `GET /api/v1/masters` | **rate-limit по IP** → 429 ожидаем |
| Карточка мастера | `GET /api/v1/masters/{slug}` | **rate-limit по IP** → 429 ожидаем |

Реальные слаги/ID тянутся один раз в `setup()` из живого каталога, поэтому
детальные ручки бьют по существующим записям. Если каталог пуст — засей
demo-данные:

```bash
psql "$VENUE_DB_URL" -f deploy/seed/demo_venues.sql
psql "$MASTER_DB_URL" -f deploy/seed/demo_masters.sql
```

## Установка k6

```bash
brew install k6                       # macOS
# или: docker run --rm -i grafana/k6 ...   (см. ниже)
```

## Запуск

```bash
# из корня репозитория
make loadtest                                  # профиль load против localhost:8080

# или напрямую, с выбором профиля и адреса
BASE_URL=http://localhost:8080 PROFILE=smoke  k6 run deploy/loadtest/public-read.js
BASE_URL=http://localhost:8080 PROFILE=load   k6 run deploy/loadtest/public-read.js
BASE_URL=http://localhost:8080 PROFILE=stress k6 run deploy/loadtest/public-read.js
BASE_URL=http://localhost:8080 PROFILE=spike  k6 run deploy/loadtest/public-read.js
```

Через Docker (без локального k6), в сети compose-стека:

```bash
docker run --rm -i --network banya-net \
  -e BASE_URL=http://api-gateway:8080 -e PROFILE=load \
  -v "$PWD/deploy/loadtest:/scripts" grafana/k6 run /scripts/public-read.js
```

### Профили

| PROFILE | Нагрузка | Зачем |
|---|---|---|
| `smoke` | 1 VU, 30s | проверить, что сценарий работает |
| `load` *(default)* | разгон до 50 VU, ~5 мин | базовая нагрузка |
| `stress` | до 300 VU | найти точку деградации |
| `spike` | всплеск до 400 VU | поведение на пиковом заходе |

## Пороги (thresholds)

Прогон падает (exit code ≠ 0), если нарушено — удобно для CI:

- `http_req_failed`: доля ошибок `< 1%` (429 от rate-limit мастеров **исключены** — они в отдельной метрике `rl_429`);
- `http_req_duration` (успешные): `p95 < 500ms`, `p99 < 1500ms`;
- `detail_latency` (карточки): `p95 < 600ms`;
- `checks`: `> 99%` успешных проверок.

Правь пороги под свой SLO прямо в `options.thresholds`.

## Экспорт метрик в Grafana

В репо уже есть Prometheus + Grafana (`deploy/docker-compose.observability.yml`).
Чтобы толкать метрики самого k6 в этот Prometheus, включи на нём приёмник
remote-write (добавь флаг в команду сервиса `prometheus`):

```yaml
# deploy/docker-compose.observability.yml → services.prometheus.command
- --web.enable-remote-write-receiver
```

и запускай k6 с выводом в Prometheus:

```bash
K6_PROMETHEUS_RW_SERVER_URL=http://localhost:9090/api/v1/write \
  k6 run -o experimental-prometheus-rw deploy/loadtest/public-read.js
```

Независимо от k6-метрик, сам gateway во время прогона отдаёт RED-метрики на
внутреннем `METRICS_ADDR`, которые Prometheus уже скрейпит (job `api-gateway`) —
смотри дашборд **api-gateway-overview** в Grafana (http://localhost:3001).

## Куда расширять

Следующие сценарии кладём соседними файлами в этой папке:
`booking-flow.js` (auth → создание брони), `chat-ws.js` (WebSocket-нагрузка),
`mixed.js` (80% чтение + 20% авторизованные действия).
