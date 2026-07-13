// Нагрузочный тест публичного (без авторизации) каталожного трафика api-gateway.
//
// Модель нагрузки — «посетитель листает каталог»:
//   1. открывает список заведений          GET /venues
//   2. ищет по городу/типу                  GET /venues/search
//   3. смотрит «популярные города»          GET /analytics/popular-cities
//   4. открывает карточку заведения         GET /venues/{slug}
//   5. листает мастеров                      GET /masters        (RL по IP, см. ниже)
//   6. открывает карточку мастера            GET /masters/{slug} (RL по IP, см. ниже)
//   7. читает отзывы заведения               GET /venues/{venueId}/reviews
//
// Слаги/ID реальных сущностей вытягиваются один раз в setup() из живого каталога,
// чтобы карточки открывались по существующим записям, а не по 404.
//
// ── Запуск ────────────────────────────────────────────────────────────────
//   k6 run deploy/loadtest/public-read.js
//   BASE_URL=http://localhost:8080 PROFILE=smoke   k6 run deploy/loadtest/public-read.js
//   BASE_URL=https://api.example.ru PROFILE=stress k6 run deploy/loadtest/public-read.js
//
// Профили нагрузки выбираются через env PROFILE (по умолчанию `load`):
//   smoke  — 1 VU, 30s: дымовой прогон, проверка что сценарий вообще работает
//   load   — плавный разгон до 50 VU, ~5 мин: базовая проверка под нагрузкой
//   stress — разгон до 300 VU: ищем точку деградации
//   spike  — резкий всплеск до 400 VU: поведение на пиковом заходе
//
// ── Замечание про rate-limit мастеров ──────────────────────────────────────
// GET /masters и /masters/{slug} на gateway защищены RateLimit по IP
// (masterPublicRL, fail-open). Один генератор нагрузки = один IP = один ключ,
// поэтому под нагрузкой эти эндпоинты закономерно начнут отвечать 429. Это НЕ
// баг сценария: 429 на этих ручках считаем ожидаемым и выносим в отдельную
// метрику `rl_429`, а из общего порога ошибок исключаем (см. thresholds).

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Trend } from 'k6/metrics';
import { randomItem } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const BASE_URL = (__ENV.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const API = `${BASE_URL}/api/v1`;
const PROFILE = __ENV.PROFILE || 'load';

// Отдельные метрики, чтобы в сводке видеть каждую ручку по имени.
const rl429 = new Counter('rl_429'); // ожидаемые 429 от rate-limit мастеров
const detailLatency = new Trend('detail_latency', true);

// 429 на rate-limited ручках мастеров — ожидаемый ответ, а не сбой. По умолчанию
// k6 считает всё вне 2xx/3xx за http_req_failed, из-за чего 429 раздували метрику
// ошибок. Расширяем список «успешных» статусов на 429; для видимости эти ответы
// продолжают отдельно считаться в counter rl_429. Из публичных read-ручек rate-limit
// стоит только на /masters*, поэтому глобальное послабление ничего лишнего не прячет.
http.setResponseCallback(http.expectedStatuses({ min: 200, max: 399 }, 429));

// ── Профили ─────────────────────────────────────────────────────────────────
const PROFILES = {
  smoke: {
    executor: 'constant-vus',
    vus: 1,
    duration: '30s',
  },
  // Управляемый уровень для построения кривой ёмкости: держим фиксированное
  // число VU и смотрим steady-state латентность/ошибки. VUS/DURATION задаются env.
  const: {
    executor: 'constant-vus',
    vus: Number(__ENV.VUS || 100),
    duration: __ENV.DURATION || '60s',
  },
  load: {
    executor: 'ramping-vus',
    startVUs: 0,
    stages: [
      { duration: '1m', target: 50 }, // разгон
      { duration: '3m', target: 50 }, // плато
      { duration: '1m', target: 0 }, // остывание
    ],
    gracefulRampDown: '30s',
  },
  stress: {
    executor: 'ramping-vus',
    startVUs: 0,
    stages: [
      { duration: '2m', target: 100 },
      { duration: '3m', target: 300 },
      { duration: '2m', target: 300 },
      { duration: '1m', target: 0 },
    ],
    gracefulRampDown: '30s',
  },
  spike: {
    executor: 'ramping-vus',
    startVUs: 0,
    stages: [
      { duration: '30s', target: 20 },
      { duration: '20s', target: 400 }, // резкий всплеск
      { duration: '1m', target: 400 },
      { duration: '30s', target: 0 },
    ],
    gracefulRampDown: '30s',
  },
};

if (!PROFILES[PROFILE]) {
  throw new Error(`unknown PROFILE=${PROFILE}; expected one of: ${Object.keys(PROFILES).join(', ')}`);
}

export const options = {
  scenarios: {
    public_read: { ...PROFILES[PROFILE], exec: 'browse' },
  },
  thresholds: {
    // Общая доступность: read-путь должен держать >99% успешных ответов.
    // 429 от rate-limit мастеров исключены — они в отдельной метрике rl_429.
    http_req_failed: ['rate<0.01'],
    // Латентность каталога. Настраивай под свой SLO.
    'http_req_duration{expected_response:true}': ['p(95)<500', 'p(99)<1500'],
    // Карточки (детальные ручки) — отдельный, более строгий порог.
    detail_latency: ['p(95)<600'],
    // Явный порог на долю проваленных check().
    checks: ['rate>0.99'],
  },
  // Не считаем 429 за сетевую ошибку теста.
  discardResponseBodies: false,
};

// ── setup: один раз тянем реальные слаги/ID из каталога ──────────────────────
export function setup() {
  const pool = { venueSlugs: [], venueIds: [], masterSlugs: [], cities: [] };

  const vRes = http.get(`${API}/venues?page=1&page_size=50`);
  if (vRes.status === 200) {
    const venues = (vRes.json('venues') || []);
    for (const v of venues) {
      if (v.slug) pool.venueSlugs.push(v.slug);
      if (v.id) pool.venueIds.push(v.id);
      if (v.city) pool.cities.push(v.city);
    }
  }

  const cRes = http.get(`${API}/analytics/popular-cities?limit=20`);
  if (cRes.status === 200) {
    for (const c of cRes.json('cities') || []) {
      if (c.city) pool.cities.push(c.city);
    }
  }

  const mRes = http.get(`${API}/masters?page=1&page_size=50`);
  if (mRes.status === 200) {
    // ListPublic отдаёт каталог; ключ массива — masters/items, берём оба варианта.
    const masters = mRes.json('masters') || mRes.json('items') || [];
    for (const m of masters) {
      if (m.slug) pool.masterSlugs.push(m.slug);
    }
  }

  // Дедуп городов.
  pool.cities = [...new Set(pool.cities)];

  console.log(
    `setup: venues=${pool.venueSlugs.length} venueIds=${pool.venueIds.length} ` +
      `masters=${pool.masterSlugs.length} cities=${pool.cities.length}`,
  );
  if (pool.venueSlugs.length === 0) {
    console.warn('setup: каталог заведений пуст — засей demo-данные (deploy/seed/demo_venues.sql), иначе детальные ручки дадут 404');
  }
  return pool;
}

// Обёртка: единый check + учёт ожидаемых 429 на rate-limited ручках.
function hit(name, res, { rateLimited = false } = {}) {
  if (rateLimited && res.status === 429) {
    rl429.add(1, { name });
    return;
  }
  check(res, {
    [`${name}: status 200`]: (r) => r.status === 200,
  });
}

// ── Основной сценарий: одна итерация = одна «сессия просмотра» ────────────────
export function browse(pool) {
  // 1. Список заведений (с пагинацией и сортировкой).
  const page = 1 + Math.floor(Math.random() * 3);
  const sort = randomItem(['', 'rating', 'price', 'newest']);
  hit(
    'venues_list',
    http.get(`${API}/venues?page=${page}&page_size=20${sort ? `&sort_by=${sort}` : ''}`, {
      tags: { name: 'venues_list' },
    }),
  );

  // 2. Поиск по городу/типу.
  const city = pool.cities.length ? randomItem(pool.cities) : 'Москва';
  const type = randomItem(['', 'banya', 'sauna', 'hammam']);
  hit(
    'venues_search',
    http.get(
      `${API}/venues/search?city=${encodeURIComponent(city)}${type ? `&type=${type}` : ''}&page=1&page_size=20`,
      { tags: { name: 'venues_search' } },
    ),
  );

  // 3. Популярные города (лёгкая, часто дёргаемая ручка на главной).
  hit('popular_cities', http.get(`${API}/analytics/popular-cities?limit=6`, { tags: { name: 'popular_cities' } }));

  // 4. Карточка заведения по реальному слагу.
  if (pool.venueSlugs.length) {
    const slug = randomItem(pool.venueSlugs);
    const r = http.get(`${API}/venues/${encodeURIComponent(slug)}`, { tags: { name: 'venue_detail' } });
    detailLatency.add(r.timings.duration, { name: 'venue_detail' });
    hit('venue_detail', r);
  }

  // 5. Отзывы заведения.
  if (pool.venueIds.length) {
    const id = randomItem(pool.venueIds);
    // review-service требует page ∈ [1,1000] — шлём пагинацию явно.
    hit(
      'venue_reviews',
      http.get(`${API}/venues/${encodeURIComponent(id)}/reviews?page=1&page_size=20`, {
        tags: { name: 'venue_reviews' },
      }),
    );
  }

  // 6. Список мастеров — rate-limited по IP, 429 ожидаем.
  hit('masters_list', http.get(`${API}/masters?page=1&page_size=20`, { tags: { name: 'masters_list' } }), {
    rateLimited: true,
  });

  // 7. Карточка мастера — тоже rate-limited.
  if (pool.masterSlugs.length) {
    const slug = randomItem(pool.masterSlugs);
    const r = http.get(`${API}/masters/${encodeURIComponent(slug)}`, { tags: { name: 'master_detail' } });
    if (r.status !== 429) detailLatency.add(r.timings.duration, { name: 'master_detail' });
    hit('master_detail', r, { rateLimited: true });
  }

  // Пауза между «страницами» — имитируем think-time реального посетителя.
  sleep(Math.random() * 1.5 + 0.5);
}
