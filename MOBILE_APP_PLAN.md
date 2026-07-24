# Мобильное приложение БаняГид — план работ (хендофф)

> Цель: превратить веб-агрегатор (Next.js SSR, `frontend/`) в приложение для iOS/Android.
> Подход: **Capacitor** (нативная оболочка грузит сайт по `server.url`), НЕ React Native.
> Причина: один код на веб и приложение; сайт ещё дорабатывается; для агрегатора WebView-UX достаточен.

## Ключевое архитектурное решение
- **Фронтенд НЕ переписываем.** Кривой мобильный вид — это недоделанный адаптив, а не проблема подхода.
- Чиним **адаптивную верстку в общем коде** → выигрывает и веб (мобильный трафик/SEO), и приложение.
- «Отдельный фронт для приложения» / RN — отвергнуто: ×2 работы, потеря SSR/SEO/логики, та же адаптивная работа всё равно нужна.

---

## Что уже сделано

### Capacitor
- Установлены `@capacitor/core|cli|ios|android` **8.4.2** в `frontend/`.
- `frontend/capacitor.config.ts`:
  - `appId: "io.banya.app"` — **ПЛЕЙСХОЛДЕР**, поменять на реальный bundle ID до публикации (потом менять нельзя).
  - `server.url` управляется env `CAP_SERVER_URL` (по умолчанию `https://example.com`).
  - `allowNavigation` по хосту (чтобы клики не улетали в Safari), `cleartext` включается для http.
- Нативные проекты: `frontend/ios/`, `frontend/android/`.
- npm-скрипты: `cap:sync`, `cap:ios`, `cap:android`.
- Fallback-экран офлайна: `frontend/mobile/www/index.html`.

### Данные окружения (dev)
- Mac LAN IP: **192.168.0.101**, докер фронта на **:3000** (`CAP_SERVER_URL=http://192.168.0.101:3000`).
- iPhone 15 Pro Max, **hardware UDID `00008130-000458C922E1001C`** (для `cap run --target` нужен именно он, не pairing-UUID).
- Подпись: Apple ID `tienlao004@gmail.com`, Team **`JH966L4JW9`** (Personal Team, бесплатный — приложение протухает через 7 дней).

### Критические фиксы (корневые причины, уже в коде)
1. **`frontend/ios/App/App/AppDelegate.swift`** — вызов `continue userActivity` через протокол `UIApplicationDelegate` (иначе не собирается под Xcode 16: перегрузка `ApplicationDelegateProxy` не резолвится).
2. **`frontend/src/lib/csp.ts`** — `style-src` теперь включает `'nonce-…'` (+ `style-src-attr 'unsafe-inline'`). WebKit блокирует `<link>`/`<style>` с nonce, если nonce нет в политике → сайт был без стилей в Safari/iOS.
3. **`frontend/src/lib/csp.ts` + `frontend/src/proxy.ts`** — `upgrade-insecure-requests` теперь **только для HTTPS** (`opts.secure`, вычисляется по `x-forwarded-proto`/протоколу). **Это была главная причина**: на http-сервере директива апгрейдила все CSS/JS в https → пустая страница без стилей и без JS в WebView.
4. **`frontend/src/app/globals.css`** — `html,body { overflow-x: clip }` (глушит горизонтальный скролл глобально, `clip` не ломает `position: sticky`) + `body { overflow-wrap: break-word }` (перенос длинных строк).
5. **`frontend/src/app/layout.tsx`** — `export const viewport = { viewportFit: "cover" }` (иначе не работают `env(safe-area-inset-*)`).
6. **`frontend/src/components/banya/header.tsx`** — `pt-[env(safe-area-inset-top)]` (шапка ниже статус-бара/чёлки).
7. **`frontend/src/components/banya/master-card.tsx`** — `min-w-0` на грид-элемент (карточка распирала колонку через `min-width: auto`). ⚠️ **Внесено, но НЕ пересобрано/не проверено.**

---

## Текущее состояние
- Приложение установлено на iPhone, контент грузится, **стили работают**, JS работает.
- Шапка (safe-area) починена, горизонтальный скролл глобально погашен.
- Осталось: **постраничная доводка адаптива**. Фикс `master-card` ждёт пересборки.
- ⚠️ Телефон часто показывает **кеш** (WKWebView) — после правок нужен чистый реинстол, чтобы увидеть реальный результат.

---

## Оставшаяся работа: сплошной проход по адаптиву

Идти постранично на ширине **430px** (и проверять 375px), искать overflow/битые лейауты, чинить в общем коде.

### Приоритеты
| Группа | Кол-во | Приоритет |
|---|---|---|
| Публичные (юзер/приложение) | ~24 | 🔴 высокий |
| Auth | 6 | 🔴 высокий |
| Кабинет владельца (CRM) | 20 | 🟡 средний |
| Админка | 4 | ⚪ низкий |

### Статус страниц
- ✅ Чисто (проверено): `/`, `/venues`, `/venues/[slug]`, `/venues/city/*`.
- 🔧 Правится: `/masters` (фикс внесён, нужна пересборка+проверка).
- ❓ Не проверены: `/masters/[slug]`, `/my/*`, `/auth/*`, `/partner*`, `/about`, `/support`, `/legal/*`, весь `owner/*`, `admin/*`.

### Отдельная задача «выглядит как сайт, а не приложение»
- Прятать **веб-футер** и лишнюю веб-шапку, когда открыто в приложении: `Capacitor.isNativePlatform()` → условный рендер в `AppLayout` (`frontend/src/app/app-layout.tsx`).
- Опционально: нижний таб-бар для режима приложения (можно через группу роутов `app/(mobile)/` в том же Next.js, без второго фронта).

---

## Рабочий процесс и команды

### Быстрый dev-loop для адаптива (рекомендуется)
Вместо пересборки докера на каждую правку — поднять dev-сервер с HMR:
```bash
cd frontend && npm run dev        # (при занятом 3000 — на другом порту, напр. 3001)
```
Правки CSS/TSX видны мгновенно. Проверять в браузере на 430px.

### Диагностика overflow (в консоли браузера на 430px)
Найти виновника и пройти вверх по дереву до элемента с `min-width: auto`/фиксированной шириной → добавить `min-w-0` / убрать фикс. Сниппет-обход дерева искал элементы с `getBoundingClientRect().right > clientWidth`.

### Пересборка докер-фронта (когда нужно прод-подобное)
```bash
cd deploy && docker compose --profile frontend build frontend \
  && docker compose --profile frontend up -d --no-deps frontend   # ВСЕГДА --no-deps
```

### Установка на iPhone
1. Подключить USB, разблокировать, Developer Mode вкл. Дождаться статуса `connected` (DDI смонтирован), не `no DDI`/`connecting`:
   ```bash
   xcrun devicectl list devices | grep -i iphone
   ```
2. Установить:
   ```bash
   cd frontend && CAP_SERVER_URL=http://192.168.0.101:3000 \
     npx cap run ios --target 00008130-000458C922E1001C
   ```
3. `cap run` часто пишет `error launching app on device` — **это норма** (приложение ставится, запускать вручную с домашнего экрана).
4. Чистый реинстол (сброс кеша WebView): сначала `xcrun devicectl device uninstall app --device 00008130-000458C922E1001C io.banya.app`, потом `cap run`.

### Когда что нужно
- Правка **веб-контента/CSS** → пересборка докера (или dev) + переоткрыть приложение (иногда реинстол ради кеша). Переустановка native НЕ нужна.
- Правка **`capacitor.config.ts` / нативки / иконки** → `cap sync` + переустановка на телефон (USB).

---

## Грабли (чтобы не наступить снова)
- `server.url` по **http** + `upgrade-insecure-requests` = всё ломается (пофикшено, но помнить при смене окружения).
- WKWebView **агрессивно кеширует** → если правка «не видна», делай чистый реинстол.
- `cap run --target` хочет **hardware UDID**, не coredevice-UUID.
- Свежий iOS-runtime симулятора грузится ~7 ГБ; при **забитом диске** первый boot симулятора падает с `Data Migration Failed` (чинится `simctl erase`). Держать диск свободным.
- `overflow-x: clip` (не `hidden`) — чтобы не сломать `position: sticky`.
- Диск чистить безопасно: `docker builder prune -f`, `docker image prune -f`, `npm cache clean --force`. **НЕ трогать** `docker volume prune` — там БД Postgres.

---

## TODO до публикации в сторы
- [ ] Реальный **HTTPS-домен** прод-фронта (тогда `upgrade-insecure-requests` работает корректно и WebView грузит нормально).
- [ ] Реальный **`appId`/bundle ID** (сейчас плейсхолдер `io.banya.app`).
- [ ] Иконка + сплэш (сейчас дефолтные Capacitor) — плагин `@capacitor/assets`.
- [x] **Push-уведомления (код готов, FCM для обеих платформ).** См. раздел ниже — осталась только нативная конфигурация на аккаунтах Firebase/Apple.
- [ ] Аккаунты: Apple Developer ($99/год), Google Play ($25 разово).
- [ ] Беспроводная отладка (Xcode → Devices → Connect via network), чтобы ставить без кабеля.

---

## Push-уведомления (FCM) — что сделано и что осталось

**Провайдер:** Firebase Cloud Messaging для iOS **и** Android (единый токен и код-путь).

### Готово в коде (этот PR)
- **Бэкенд `notification-service`:** таблица `device_tokens` (миграция `002`),
  RPC `RegisterDevice`/`UnregisterDevice`, FCM HTTP v1 sender
  (`internal/fcm`). Пуш шлётся автоматически в `usecase.Create` — через ту же
  воронку, что и веб-колокольчик (брони, отзывы, модерация, инвайты). Мёртвые
  токены (HTTP 404) чистятся сами.
- **Gateway:** `POST /api/v2/notifications/devices` (регистрация токена),
  `DELETE …/devices` (снятие).
- **Фронт:** плагин `@capacitor-firebase/messaging`; компонент
  `PushRegistration` (в `providers.tsx`) на нативе запрашивает разрешение,
  берёт FCM-токен, шлёт на бэкенд, роутит тап по уведомлению; на выходе из
  аккаунта токен снимается.
- **Конфиг:** `FCM_CREDENTIALS_JSON` (или `FCM_CREDENTIALS_FILE`) в
  `notification-service`. **Пусто ⇒ пуши выключены**, колокольчик работает.

### Осталось (заблокировано на ваших аккаунтах Firebase/Apple)
1. Завести **проект Firebase**, добавить в него iOS- и Android-приложения с
   реальным bundle ID.
2. **Android:** положить `google-services.json` в `frontend/android/app/`.
3. **iOS:** положить `GoogleService-Info.plist` в `frontend/ios/App/App/`,
   включить capability **Push Notifications** + **Background Modes → Remote
   notifications**, загрузить **APNs-ключ (.p8)** в Firebase (Cloud Messaging).
4. Скачать из Firebase **service-account JSON** → положить в `deploy/.env` как
   `FCM_CREDENTIALS_JSON='{…}'`.
5. `cd frontend && npx cap sync` (подтянет нативный плагин + Pods), пересобрать
   приложение.
6. Применить миграцию: `bash deploy/migrate.sh` (создаст `device_tokens`).

---

## Рекомендуемый порядок в новом чате
1. Пересобрать докер (или поднять dev), **чистый реинстол на телефон** — увидеть реальное состояние (шапка+скролл уже ок).
2. Завести быстрый dev-loop, пройти **публичные + auth** страницы по адаптиву (430px), чинить точечно (`min-w-0`, стек колонок, переносы).
3. Сделать фронт **app-aware**: прятать футер/веб-шапку в приложении (`Capacitor.isNativePlatform()`).
4. Кабинет владельца (`owner/*`) — адаптив.
5. Иконка/сплэш, потом прод-домен + подготовка к сторам.
