-- Демо-сид для master-service. Город — Иваново.
-- Запуск:
--   docker exec -i banya-postgres psql -U banya -d master_db < deploy/seed/demo_masters.sql

BEGIN;

INSERT INTO masters (id, user_id, slug, display_name, bio, phone, city, work_format, travel_radius_km, experience_years, specializations, hourly_rate, status)
VALUES
  ('44444444-4444-4444-4444-000000000001'::uuid, gen_random_uuid(), 'andrey-volkov',  'Андрей Волков',  'Парильщик в третьем поколении. Работаю с дубовыми и берёзовыми вениками. Авторская техника «уход в баню».', '+7 (920) 340-10-01', 'Иваново', 'both',   30, 12, ARRAY['veniki','contrast','aroma']::text[],    2500, 'active'),
  ('44444444-4444-4444-4444-000000000002'::uuid, gen_random_uuid(), 'mariya-ivanova', 'Мария Иванова',  'Сертифицированный массажист хамама. Пенный массаж, кесе-пилинг, ароматерапия.',                                  '+7 (920) 340-10-02', 'Иваново', 'venue',  0,  8,  ARRAY['hammam','foam_massage','aroma']::text[], 2200, 'active'),
  ('44444444-4444-4444-4444-000000000003'::uuid, gen_random_uuid(), 'pavel-sokolov',  'Павел Соколов',  'Банщик-травник. Подбираю веники и настои под запрос: расслабление, тонус, иммунитет.',                          '+7 (920) 340-10-03', 'Иваново', 'mobile', 50, 6,  ARRAY['veniki','herbs','phyto']::text[],       1800, 'active'),
  ('44444444-4444-4444-4444-000000000004'::uuid, gen_random_uuid(), 'elena-petrova',  'Елена Петрова',  'Спа-терапевт. Программы похудения, антицеллюлитные обёртывания, фитобочка.',                                    '+7 (920) 340-10-04', 'Иваново', 'venue',  0,  10, ARRAY['spa','phyto','wrap']::text[],           2000, 'active'),
  ('44444444-4444-4444-4444-000000000005'::uuid, gen_random_uuid(), 'denis-morozov',  'Денис Морозов',  'Банный мастер. Классические русские техники, чан на дровах, контрастные процедуры.',                            '+7 (920) 340-10-05', 'Иваново', 'both',   80, 15, ARRAY['veniki','chan','contrast']::text[],     3000, 'active')
ON CONFLICT (slug) DO UPDATE SET
  display_name = EXCLUDED.display_name,
  bio = EXCLUDED.bio,
  phone = EXCLUDED.phone,
  city = EXCLUDED.city,
  work_format = EXCLUDED.work_format,
  travel_radius_km = EXCLUDED.travel_radius_km,
  experience_years = EXCLUDED.experience_years,
  specializations = EXCLUDED.specializations,
  hourly_rate = EXCLUDED.hourly_rate,
  status = EXCLUDED.status,
  updated_at = now();

-- По одному репрезентативному сервису на мастера.
INSERT INTO master_services (id, master_id, name, description, duration_min, price, sort_order)
VALUES
  ('55555555-5555-5555-5555-000000000001'::uuid, '44444444-4444-4444-4444-000000000001'::uuid, 'Парение с веником',     'Дубовый или берёзовый веник, 60 минут.',     60, 2500, 0),
  ('55555555-5555-5555-5555-000000000002'::uuid, '44444444-4444-4444-4444-000000000002'::uuid, 'Пенный массаж в хамаме','Кесе-пилинг + пенный массаж, 75 минут.',     75, 3000, 0),
  ('55555555-5555-5555-5555-000000000003'::uuid, '44444444-4444-4444-4444-000000000003'::uuid, 'Парение + травяной чай','Подбор веников и фиточая под запрос.',       60, 1800, 0),
  ('55555555-5555-5555-5555-000000000004'::uuid, '44444444-4444-4444-4444-000000000004'::uuid, 'Спа-обёртывание',       'Шоколад / водоросли / грязевое, 60 минут.',  60, 2000, 0),
  ('55555555-5555-5555-5555-000000000005'::uuid, '44444444-4444-4444-4444-000000000005'::uuid, 'Парение с чаном',       'Классическое парение + чан на дровах.',      90, 3500, 0)
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  description = EXCLUDED.description,
  duration_min = EXCLUDED.duration_min,
  price = EXCLUDED.price;

-- Аватарки (Unsplash-плейсхолдеры портретов).
INSERT INTO master_photos (id, master_id, url, sort_order, is_cover)
VALUES
  ('66666666-6666-6666-6666-000000000001'::uuid, '44444444-4444-4444-4444-000000000001'::uuid, 'https://images.unsplash.com/photo-1507003211169-0a1dd7228f2d?w=600', 0, true),
  ('66666666-6666-6666-6666-000000000002'::uuid, '44444444-4444-4444-4444-000000000002'::uuid, 'https://images.unsplash.com/photo-1494790108377-be9c29b29330?w=600', 0, true),
  ('66666666-6666-6666-6666-000000000003'::uuid, '44444444-4444-4444-4444-000000000003'::uuid, 'https://images.unsplash.com/photo-1500648767791-00dcc994a43e?w=600', 0, true),
  ('66666666-6666-6666-6666-000000000004'::uuid, '44444444-4444-4444-4444-000000000004'::uuid, 'https://images.unsplash.com/photo-1438761681033-6461ffad8d80?w=600', 0, true),
  ('66666666-6666-6666-6666-000000000005'::uuid, '44444444-4444-4444-4444-000000000005'::uuid, 'https://images.unsplash.com/photo-1472099645785-5658abf4ff4e?w=600', 0, true)
ON CONFLICT (id) DO UPDATE SET url = EXCLUDED.url;

COMMIT;
