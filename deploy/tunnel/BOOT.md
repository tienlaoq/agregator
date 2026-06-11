# Boot-инструкция: фестивальный демо banyaorganica.ru

Минутный чек-лист для дня выставки. Всё запускается на этом ноутбуке;
домен `banyaorganica.ru` ведёт сюда через Cloudflare Tunnel.

## Однократно (до фестиваля)

1. **Cloudflare Tunnel**. Скрипт всё делает сам, один клик в браузере:
   ```bash
   bash deploy/tunnel/setup.sh
   ```
   Откроется страница Cloudflare — выбери зону `banyaorganica.ru`, нажми
   Authorize. Скрипт пропишет DNS, сгенерит `deploy/tunnel/config.yml`.

## Каждый день фестиваля (порядок важен)

Все команды из корня репо (`cd /Users/tienlao/agregator`).

```bash
# 1. Инфра (postgres, redis, nats, minio)
make infra-up

# 2. Бэкенд + frontend (с фестивальными env)
docker compose -f deploy/docker-compose.yml \
    --env-file deploy/.env --env-file deploy/.env.demo \
    --profile frontend up -d \
    auth-service user-service venue-service booking-service \
    master-service chat-service api-gateway frontend

# 3. Туннель (foreground — пусть висит в отдельном окне терминала)
cloudflared tunnel --config deploy/tunnel/config.yml run
```

Открывай в браузере: <https://app.banyaorganica.ru>

## Проверка перед открытием стенда (~30 сек)

- `https://app.banyaorganica.ru` → главная грузится
- `https://app.banyaorganica.ru/venues?city=Иваново` → 15 бань на карте
- Открой любую карточку → фото, описание, услуги
- Кнопка «Войти через ВК» → редирект → возврат на сайт залогиненным
- `https://api.banyaorganica.ru/api/v1/venues?city=Иваново&limit=1` → JSON

## Завершение (после фестиваля)

```bash
# Ctrl+C в окне с cloudflared
docker compose -f deploy/docker-compose.yml --profile frontend down
make infra-down
```

Туннель и DNS-записи остаются — поднимутся той же командой `cloudflared run`.
Если хочешь снести полностью:

```bash
cloudflared tunnel delete banyaorganica-demo
# DNS-записи app.* / api.* в дашборде Cloudflare удали вручную.
```

## Если что-то сломалось

| Симптом | Куда смотреть |
|---|---|
| Сайт открывается, но «502 Bad Gateway» | `docker ps` — поднят ли frontend; `docker logs banya-frontend` |
| API отвечает CORS-ошибкой | `docker logs banya-gateway \| head -30` → строка `CORS allowlist` должна содержать `https://app.banyaorganica.ru` |
| Карта без точек | `curl https://api.banyaorganica.ru/api/v1/venues?city=Иваново` — пуст? значит сиды не залились, выполни `docker exec -i banya-postgres psql -U banya -d venue_db < deploy/seed/demo_venues.sql` |
| `cloudflared` ругается на cert | `rm ~/.cloudflared/cert.pem && bash deploy/tunnel/setup.sh` |

## Что НЕ работает в этом демо (специально)

- Реальные платежи через ЮKassa (payment-service не подключён к боевому контуру)
- Email/SMS-уведомления (SMTP/Telegram не настроены)
- Отзывы (review-service есть, но пустой)
- Админ-CRM (доступ только локально)
