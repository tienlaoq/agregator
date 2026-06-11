-- Демо-сид для венчур-сервиса. Город — Иваново.
-- Координаты — точки в радиусе ~5км от центра (57.0058, 40.9739).
-- Запускается из контейнера postgres:
--   docker exec -i banya-postgres psql -U banya -d venue_db < deploy/seed/demo_venues.sql
-- Идемпотентно: ON CONFLICT по slug.

BEGIN;

INSERT INTO venues (id, owner_id, slug, name, type, description, address, location, price_from, capacity, amenities, working_hours, phone, status, city, is_active)
VALUES
  ('11111111-1111-1111-1111-000000000001'::uuid, gen_random_uuid(), 'banya-na-lezhnevskoy',  'Баня на Лежневской',        'banya',     'Классическая русская баня на дровах. 2 парные, бассейн с холодной водой, веники в подарок.', 'г. Иваново, ул. Лежневская, 114', ST_SetSRID(ST_MakePoint(40.9851, 56.9991), 4326)::geography,  2500, 8,  ARRAY['parking','wifi','pool','venik']::text[],     '{"mon":"10:00-23:00","tue":"10:00-23:00","wed":"10:00-23:00","thu":"10:00-23:00","fri":"10:00-23:00","sat":"00:00-24:00","sun":"00:00-24:00"}'::jsonb, '+7 (4932) 30-00-01', 'active', 'Иваново', true),
  ('11111111-1111-1111-1111-000000000002'::uuid, gen_random_uuid(), 'sauna-uvod',           'Сауна «Увод»',              'sauna',     'Финская сауна на берегу Увода. Купель, комната отдыха, мангальная зона.',                    'г. Иваново, наб. Уводь, 12',     ST_SetSRID(ST_MakePoint(41.0011, 57.0150), 4326)::geography,  1800, 6,  ARRAY['parking','grill','river']::text[],            '{"mon":"12:00-24:00","tue":"12:00-24:00","wed":"12:00-24:00","thu":"12:00-24:00","fri":"12:00-24:00","sat":"00:00-24:00","sun":"00:00-24:00"}'::jsonb, '+7 (4932) 30-00-02', 'active', 'Иваново', true),
  ('11111111-1111-1111-1111-000000000003'::uuid, gen_random_uuid(), 'hammam-vostok',         'Хамам «Восток»',            'hammam',    'Турецкий хамам с мраморным лежаком, пилинг кесе и пенный массаж.',                            'г. Иваново, пр. Ленина, 25',     ST_SetSRID(ST_MakePoint(40.9742, 57.0010), 4326)::geography,  3200, 4,  ARRAY['wifi','massage','tea_ceremony']::text[],      '{"mon":"10:00-22:00","tue":"10:00-22:00","wed":"10:00-22:00","thu":"10:00-22:00","fri":"10:00-22:00","sat":"10:00-22:00","sun":"10:00-22:00"}'::jsonb, '+7 (4932) 30-00-03', 'active', 'Иваново', true),
  ('11111111-1111-1111-1111-000000000004'::uuid, gen_random_uuid(), 'fito-zdorove',          'Фитобочка «Здоровье»',      'fitobochka','Кедровая фитобочка с травяным сбором. Курс 5/10 сеансов.',                                   'г. Иваново, ул. Куконковых, 5',  ST_SetSRID(ST_MakePoint(40.9510, 56.9850), 4326)::geography,  600,  1,  ARRAY['wifi','herbs']::text[],                       '{"mon":"09:00-21:00","tue":"09:00-21:00","wed":"09:00-21:00","thu":"09:00-21:00","fri":"09:00-21:00","sat":"10:00-20:00","sun":"10:00-20:00"}'::jsonb, '+7 (4932) 30-00-04', 'active', 'Иваново', true),
  ('11111111-1111-1111-1111-000000000005'::uuid, gen_random_uuid(), 'banya-merkury',         'Баня «Меркурий»',           'banya',     'Большой банный комплекс: парная, хамам, бассейн, бар, караоке.',                              'г. Иваново, ул. 8 Марта, 32',    ST_SetSRID(ST_MakePoint(40.9920, 57.0083), 4326)::geography,  4500, 12, ARRAY['parking','wifi','pool','bar','karaoke']::text[],'{"mon":"00:00-24:00","tue":"00:00-24:00","wed":"00:00-24:00","thu":"00:00-24:00","fri":"00:00-24:00","sat":"00:00-24:00","sun":"00:00-24:00"}'::jsonb, '+7 (4932) 30-00-05', 'active', 'Иваново', true),
  ('11111111-1111-1111-1111-000000000006'::uuid, gen_random_uuid(), 'banya-derevenskaya',    'Деревенская баня',          'banya',     'Срубовая баня на дровах за городом, рядом пруд, есть гостевой дом.',                          'Ивановская обл., д. Беляницы',   ST_SetSRID(ST_MakePoint(40.8950, 57.0510), 4326)::geography,  3000, 10, ARRAY['parking','pond','guesthouse','venik']::text[],'{"mon":"10:00-23:00","tue":"10:00-23:00","wed":"10:00-23:00","thu":"10:00-23:00","fri":"10:00-23:00","sat":"10:00-23:00","sun":"10:00-23:00"}'::jsonb, '+7 (4932) 30-00-06', 'active', 'Иваново', true),
  ('11111111-1111-1111-1111-000000000007'::uuid, gen_random_uuid(), 'sauna-loft',            'Сауна Loft',                'sauna',     'Современная сауна-лофт в центре. Бар, кальянная, проектор.',                                  'г. Иваново, ул. Красной Армии, 1',ST_SetSRID(ST_MakePoint(40.9802, 57.0061), 4326)::geography,  2200, 8,  ARRAY['wifi','bar','hookah','projector']::text[],    '{"mon":"14:00-02:00","tue":"14:00-02:00","wed":"14:00-02:00","thu":"14:00-02:00","fri":"14:00-04:00","sat":"14:00-04:00","sun":"14:00-02:00"}'::jsonb, '+7 (4932) 30-00-07', 'active', 'Иваново', true),
  ('11111111-1111-1111-1111-000000000008'::uuid, gen_random_uuid(), 'banya-na-talke',        'Баня на Талке',             'banya',     'Тихое место у реки Талка. Дровяная печь, чан под открытым небом.',                            'г. Иваново, ул. Талка, 7',       ST_SetSRID(ST_MakePoint(41.0210, 57.0185), 4326)::geography,  2800, 6,  ARRAY['parking','river','chan','venik']::text[],     '{"mon":"10:00-23:00","tue":"10:00-23:00","wed":"10:00-23:00","thu":"10:00-23:00","fri":"10:00-23:00","sat":"10:00-23:00","sun":"10:00-23:00"}'::jsonb, '+7 (4932) 30-00-08', 'active', 'Иваново', true),
  ('11111111-1111-1111-1111-000000000009'::uuid, gen_random_uuid(), 'hammam-bosfor',         'Хамам «Босфор»',            'hammam',    'Хамам в восточном стиле, мыльный массаж, чайная церемония.',                                   'г. Иваново, ул. Громобоя, 13',   ST_SetSRID(ST_MakePoint(40.9601, 57.0095), 4326)::geography,  2900, 4,  ARRAY['wifi','massage','tea_ceremony']::text[],      '{"mon":"11:00-23:00","tue":"11:00-23:00","wed":"11:00-23:00","thu":"11:00-23:00","fri":"11:00-23:00","sat":"11:00-23:00","sun":"11:00-23:00"}'::jsonb, '+7 (4932) 30-00-09', 'active', 'Иваново', true),
  ('11111111-1111-1111-1111-000000000010'::uuid, gen_random_uuid(), 'banya-staraya-gvardiya','Баня «Старая Гвардия»',     'banya',     'Семейная баня с историей. Два зала, бильярд, домашняя кухня.',                                 'г. Иваново, ул. Парижской Коммуны, 18', ST_SetSRID(ST_MakePoint(40.9685, 57.0150), 4326)::geography, 3500, 10, ARRAY['parking','wifi','billiard','kitchen','venik']::text[], '{"mon":"10:00-24:00","tue":"10:00-24:00","wed":"10:00-24:00","thu":"10:00-24:00","fri":"10:00-24:00","sat":"10:00-24:00","sun":"10:00-24:00"}'::jsonb, '+7 (4932) 30-00-10', 'active', 'Иваново', true),
  ('11111111-1111-1111-1111-000000000011'::uuid, gen_random_uuid(), 'sauna-aqua',            'Сауна Aqua',                'sauna',     'Сауна с большим бассейном, гидромассаж, водопад.',                                             'г. Иваново, ул. Минская, 100',   ST_SetSRID(ST_MakePoint(40.9405, 56.9710), 4326)::geography,  2000, 6,  ARRAY['parking','pool','hydromassage']::text[],      '{"mon":"10:00-24:00","tue":"10:00-24:00","wed":"10:00-24:00","thu":"10:00-24:00","fri":"10:00-24:00","sat":"10:00-24:00","sun":"10:00-24:00"}'::jsonb, '+7 (4932) 30-00-11', 'active', 'Иваново', true),
  ('11111111-1111-1111-1111-000000000012'::uuid, gen_random_uuid(), 'fito-kedr',             'Фитобочка «Кедр»',          'fitobochka','Кедровая бочка + соляная комната. Программы похудения и детокс.',                              'г. Иваново, ул. Велижская, 8',   ST_SetSRID(ST_MakePoint(41.0050, 56.9920), 4326)::geography,  700,  1,  ARRAY['wifi','salt_room','herbs']::text[],           '{"mon":"09:00-21:00","tue":"09:00-21:00","wed":"09:00-21:00","thu":"09:00-21:00","fri":"09:00-21:00","sat":"10:00-20:00","sun":"10:00-20:00"}'::jsonb, '+7 (4932) 30-00-12', 'active', 'Иваново', true),
  ('11111111-1111-1111-1111-000000000013'::uuid, gen_random_uuid(), 'banya-volgar',          'Баня «Волгарь»',            'banya',     'Баня для большой компании: 3 этажа, 2 парные, караоке, мангал.',                                'г. Иваново, ул. Окуловой, 75',   ST_SetSRID(ST_MakePoint(40.9550, 57.0250), 4326)::geography,  5000, 16, ARRAY['parking','wifi','grill','karaoke','venik']::text[],'{"mon":"00:00-24:00","tue":"00:00-24:00","wed":"00:00-24:00","thu":"00:00-24:00","fri":"00:00-24:00","sat":"00:00-24:00","sun":"00:00-24:00"}'::jsonb, '+7 (4932) 30-00-13', 'active', 'Иваново', true),
  ('11111111-1111-1111-1111-000000000014'::uuid, gen_random_uuid(), 'sauna-skandi',          'Сауна «Сканди»',            'sauna',     'Скандинавский стиль, ароматерапия, чай с травами.',                                            'г. Иваново, ул. Шубиных, 30',    ST_SetSRID(ST_MakePoint(40.9890, 57.0210), 4326)::geography,  1900, 6,  ARRAY['wifi','aroma','tea_ceremony']::text[],        '{"mon":"12:00-24:00","tue":"12:00-24:00","wed":"12:00-24:00","thu":"12:00-24:00","fri":"12:00-24:00","sat":"12:00-24:00","sun":"12:00-24:00"}'::jsonb, '+7 (4932) 30-00-14', 'active', 'Иваново', true),
  ('11111111-1111-1111-1111-000000000015'::uuid, gen_random_uuid(), 'banya-rodnik',          'Баня «Родник»',             'banya',     'Баня с природным родником и чаном на дровах. За городом, тишина.',                              'Ивановская обл., с. Иванково',   ST_SetSRID(ST_MakePoint(40.8810, 57.0680), 4326)::geography,  3200, 8,  ARRAY['parking','spring','chan','venik']::text[],    '{"mon":"10:00-23:00","tue":"10:00-23:00","wed":"10:00-23:00","thu":"10:00-23:00","fri":"10:00-23:00","sat":"10:00-23:00","sun":"10:00-23:00"}'::jsonb, '+7 (4932) 30-00-15', 'active', 'Иваново', true)
ON CONFLICT (slug) DO UPDATE SET
  name = EXCLUDED.name,
  description = EXCLUDED.description,
  address = EXCLUDED.address,
  location = EXCLUDED.location,
  price_from = EXCLUDED.price_from,
  capacity = EXCLUDED.capacity,
  amenities = EXCLUDED.amenities,
  working_hours = EXCLUDED.working_hours,
  phone = EXCLUDED.phone,
  status = EXCLUDED.status,
  city = EXCLUDED.city,
  is_active = EXCLUDED.is_active,
  updated_at = now();

-- Услуги: один представительский service на каждую баню.
INSERT INTO venue_services (id, venue_id, name, duration_min, price, description)
SELECT
  ('22222222-2222-2222-2222-' || lpad(row_number() OVER ()::text, 12, '0'))::uuid,
  v.id,
  CASE v.type
    WHEN 'banya'      THEN 'Парная на 2 часа'
    WHEN 'sauna'      THEN 'Сауна на 2 часа'
    WHEN 'hammam'     THEN 'Хамам + пилинг'
    WHEN 'fitobochka' THEN 'Сеанс 20 минут'
  END,
  CASE v.type
    WHEN 'banya'      THEN 120
    WHEN 'sauna'      THEN 120
    WHEN 'hammam'     THEN 60
    WHEN 'fitobochka' THEN 20
  END,
  v.price_from,
  'Базовая услуга. Дополнительные опции — на месте.'
FROM venues v
WHERE v.id BETWEEN '11111111-1111-1111-1111-000000000001'::uuid
              AND '11111111-1111-1111-1111-000000000015'::uuid
ON CONFLICT DO NOTHING;

-- Фото-обложка: Unsplash-плейсхолдеры, чтобы карточка не была пустой.
INSERT INTO venue_photos (id, venue_id, url, sort_order, is_cover)
VALUES
  ('33333333-3333-3333-3333-000000000001'::uuid, '11111111-1111-1111-1111-000000000001'::uuid, 'https://images.unsplash.com/photo-1545158535-c3f7168c28b6?w=1200', 0, true),
  ('33333333-3333-3333-3333-000000000002'::uuid, '11111111-1111-1111-1111-000000000002'::uuid, 'https://images.unsplash.com/photo-1571902943202-507ec2618e8f?w=1200', 0, true),
  ('33333333-3333-3333-3333-000000000003'::uuid, '11111111-1111-1111-1111-000000000003'::uuid, 'https://images.unsplash.com/photo-1591343395082-e120087004b4?w=1200', 0, true),
  ('33333333-3333-3333-3333-000000000004'::uuid, '11111111-1111-1111-1111-000000000004'::uuid, 'https://images.unsplash.com/photo-1600334129128-685c5582fd35?w=1200', 0, true),
  ('33333333-3333-3333-3333-000000000005'::uuid, '11111111-1111-1111-1111-000000000005'::uuid, 'https://images.unsplash.com/photo-1604881991720-f91add269bed?w=1200', 0, true),
  ('33333333-3333-3333-3333-000000000006'::uuid, '11111111-1111-1111-1111-000000000006'::uuid, 'https://images.unsplash.com/photo-1583416750470-965b2707b355?w=1200', 0, true),
  ('33333333-3333-3333-3333-000000000007'::uuid, '11111111-1111-1111-1111-000000000007'::uuid, 'https://images.unsplash.com/photo-1610824352934-c10d87b700cc?w=1200', 0, true),
  ('33333333-3333-3333-3333-000000000008'::uuid, '11111111-1111-1111-1111-000000000008'::uuid, 'https://images.unsplash.com/photo-1571417047976-1d5d0a6b3a14?w=1200', 0, true),
  ('33333333-3333-3333-3333-000000000009'::uuid, '11111111-1111-1111-1111-000000000009'::uuid, 'https://images.unsplash.com/photo-1610824352934-c10d87b700cc?w=1200', 0, true),
  ('33333333-3333-3333-3333-000000000010'::uuid, '11111111-1111-1111-1111-000000000010'::uuid, 'https://images.unsplash.com/photo-1545158535-c3f7168c28b6?w=1200', 0, true),
  ('33333333-3333-3333-3333-000000000011'::uuid, '11111111-1111-1111-1111-000000000011'::uuid, 'https://images.unsplash.com/photo-1571902943202-507ec2618e8f?w=1200', 0, true),
  ('33333333-3333-3333-3333-000000000012'::uuid, '11111111-1111-1111-1111-000000000012'::uuid, 'https://images.unsplash.com/photo-1600334129128-685c5582fd35?w=1200', 0, true),
  ('33333333-3333-3333-3333-000000000013'::uuid, '11111111-1111-1111-1111-000000000013'::uuid, 'https://images.unsplash.com/photo-1604881991720-f91add269bed?w=1200', 0, true),
  ('33333333-3333-3333-3333-000000000014'::uuid, '11111111-1111-1111-1111-000000000014'::uuid, 'https://images.unsplash.com/photo-1610824352934-c10d87b700cc?w=1200', 0, true),
  ('33333333-3333-3333-3333-000000000015'::uuid, '11111111-1111-1111-1111-000000000015'::uuid, 'https://images.unsplash.com/photo-1583416750470-965b2707b355?w=1200', 0, true)
ON CONFLICT (id) DO UPDATE SET url = EXCLUDED.url;

COMMIT;
