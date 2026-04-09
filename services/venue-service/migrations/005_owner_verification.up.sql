-- Данные для проверки существования точки и связи заявителя с бизнесом (модерация / антифрод)
ALTER TABLE venues ADD COLUMN IF NOT EXISTS legal_entity_name TEXT NOT NULL DEFAULT '';
ALTER TABLE venues ADD COLUMN IF NOT EXISTS inn VARCHAR(12) NOT NULL DEFAULT '';
ALTER TABLE venues ADD COLUMN IF NOT EXISTS ogrn VARCHAR(15) NOT NULL DEFAULT '';
ALTER TABLE venues ADD COLUMN IF NOT EXISTS public_listing_url TEXT NOT NULL DEFAULT '';
ALTER TABLE venues ADD COLUMN IF NOT EXISTS verification_note TEXT NOT NULL DEFAULT '';
