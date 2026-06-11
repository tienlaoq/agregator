export interface User {
  id: string;
  email: string;
  name: string;
  phone: string;
  role: string;
  avatar_url?: string;
  bio?: string;
}

export interface VenueService {
  id: string;
  name: string;
  description: string;
  price: number;
  /** Ответ API (api-gateway) */
  duration_min?: number;
  duration_minutes?: number;
}

/** Строка в форме владельца (создание / редактирование услуг). */
export type VenueServiceFormLine = {
  key: string;
  name: string;
  price: number;
  duration_min: number;
  description: string;
};

/** Услуга в теле POST /api/v1/venues и PATCH (без служебного `key`). */
export type VenueServiceApiItem = {
  name: string;
  duration_min: number;
  price: number;
  description: string;
};

/** Варианты длительности в селекте (значение — минуты). */
export const VENUE_SERVICE_DURATION_OPTIONS: { value: number; label: string }[] =
  [
    { value: 30, label: "30 минут" },
    { value: 60, label: "1 час" },
    { value: 90, label: "1.5 часа" },
    { value: 120, label: "2 часа" },
    { value: 150, label: "2.5 часа" },
    { value: 180, label: "3 часа" },
    { value: 240, label: "4 часа" },
  ];

export function newVenueServiceLine(): VenueServiceFormLine {
  const key =
    typeof crypto !== "undefined" && "randomUUID" in crypto
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
  return {
    key,
    name: "",
    price: 0,
    duration_min: 120,
    description: "",
  };
}

export function venueServicesToFormLines(
  services: VenueService[],
): VenueServiceFormLine[] {
  if (!services?.length) return [];
  return services.map((s) => ({
    key:
      s.id ||
      (typeof crypto !== "undefined" && "randomUUID" in crypto
        ? crypto.randomUUID()
        : `${Date.now()}-${Math.random().toString(36).slice(2, 11)}`),
    name: s.name,
    price: Number(s.price) || 0,
    duration_min: (s.duration_min ?? s.duration_minutes ?? 0) || 120,
    description: s.description ?? "",
  }));
}

/** Только заполненные по названию строки, без `key`. */
export function venueServiceLinesForApi(
  lines: VenueServiceFormLine[],
): VenueServiceApiItem[] {
  return lines
    .map((l) => ({
      name: l.name.trim(),
      duration_min: l.duration_min,
      price: Math.max(0, Math.round(Number(l.price)) || 0),
      description: l.description.trim(),
    }))
    .filter((l) => l.name.length > 0);
}

/** Фото карточки (ответ owner/public API) */
export interface VenuePhoto {
  id: string;
  url: string;
  sort_order?: number;
  is_cover?: boolean;
}

/** Фото зала (ответ API) */
export interface VenueHallPhoto {
  id: string;
  url: string;
  sort_order?: number;
  is_cover?: boolean;
}

/** Зал внутри заведения */
export interface VenueHall {
  id: string;
  name: string;
  price_from: number;
  capacity: number;
  amenities: string[];
  sort_order?: number;
  photos?: VenueHallPhoto[];
}

/** Строка зала в форме редактирования */
export interface VenueHallFormLine {
  clientKey: string;
  id?: string;
  name: string;
  price_from: number;
  capacity: number;
  amenities: string[];
  photos?: VenueHallPhoto[];
}

export function newVenueHallLine(): VenueHallFormLine {
  const clientKey =
    typeof crypto !== "undefined" && "randomUUID" in crypto
      ? crypto.randomUUID()
      : `h-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
  return {
    clientKey,
    name: "",
    price_from: 0,
    capacity: 8,
    amenities: [],
    photos: [],
  };
}

/** Элемент зала в теле PATCH/POST (как ожидает API) */
export type VenueHallApiItem = {
  id?: string;
  name: string;
  price_from: number;
  capacity: number;
  amenities: string[];
  sort_order: number;
};

/** Ключи ссылок на соцсети / мессенджеры (хранятся в API как JSON-объект). */
export type VenueSocialLinkKey =
  | "vk"
  | "telegram"
  | "instagram"
  | "facebook"
  | "whatsapp"
  | "youtube"
  | "max"
  | "other";

export type VenueSocialLinks = Record<VenueSocialLinkKey, string>;

export const DEFAULT_VENUE_SOCIAL_LINKS: VenueSocialLinks = {
  vk: "",
  telegram: "",
  instagram: "",
  facebook: "",
  whatsapp: "",
  youtube: "",
  max: "",
  other: "",
};

/** Подсказка у полей (Meta-экосистема) — дисклеймер для публикации ссылки. */
const META_EXTREMIST_DISCLAIMER_HINT =
  "Организация признана экстремистской и запрещена на территории России";

export const VENUE_SOCIAL_FIELD_DEFS: {
  key: VenueSocialLinkKey;
  label: string;
  placeholder: string;
  hint?: string;
}[] = [
  { key: "vk", label: "ВКонтакте", placeholder: "https://vk.com/..." },
  { key: "telegram", label: "Telegram", placeholder: "https://t.me/..." },
  { key: "max", label: "MAX", placeholder: "https://..." },
  {
    key: "whatsapp",
    label: "WhatsApp",
    placeholder: "https://wa.me/...",
    hint: META_EXTREMIST_DISCLAIMER_HINT,
  },
  { key: "youtube", label: "YouTube", placeholder: "https://youtube.com/..." },
  {
    key: "instagram",
    label: "Instagram",
    placeholder: "https://www.instagram.com/...",
    hint: META_EXTREMIST_DISCLAIMER_HINT,
  },
  {
    key: "facebook",
    label: "Facebook",
    placeholder: "https://www.facebook.com/...",
    hint: META_EXTREMIST_DISCLAIMER_HINT,
  },
  { key: "other", label: "Другая ссылка", placeholder: "https://..." },
];

/** Подписи для публичной карточки заведения. */
export const VENUE_SOCIAL_PUBLIC_LABELS: Record<VenueSocialLinkKey, string> = {
  vk: "ВКонтакте",
  telegram: "Telegram",
  max: "MAX",
  instagram: "Instagram",
  facebook: "Facebook",
  whatsapp: "WhatsApp",
  youtube: "YouTube",
  other: "Сайт или соцсеть",
};

export function parseVenueSocialLinks(
  raw: Venue["social_links"],
): VenueSocialLinks {
  const out = { ...DEFAULT_VENUE_SOCIAL_LINKS };
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return out;
  const o = raw as Record<string, unknown>;
  for (const k of Object.keys(out) as VenueSocialLinkKey[]) {
    const v = o[k];
    if (typeof v === "string") out[k] = v;
  }
  return out;
}

/** Убирает пробелы по краям у всех полей. */
export function trimVenueSocialLinks(sl: VenueSocialLinks): VenueSocialLinks {
  const out = { ...DEFAULT_VENUE_SOCIAL_LINKS };
  for (const k of Object.keys(out) as VenueSocialLinkKey[]) {
    out[k] = (sl[k] ?? "").trim();
  }
  return out;
}

/** Только непустые ссылки — для отправки в API. */
export function buildVenueSocialLinksPayload(
  sl: VenueSocialLinks,
): Record<string, string> {
  const t = trimVenueSocialLinks(sl);
  const out: Record<string, string> = {};
  for (const k of Object.keys(t) as VenueSocialLinkKey[]) {
    if (t[k]) out[k] = t[k];
  }
  return out;
}

export interface Venue {
  id: string;
  slug: string;
  name: string;
  type: "banya" | "sauna" | "hammam";
  description: string;
  address: string;
  city: string;
  phone: string;
  price_from: number;
  rating: number;
  review_count: number;
  image_url?: string;
  photos?: VenuePhoto[];
  halls?: VenueHall[];
  amenities: string[];
  services: VenueService[];
  owner_id: string;
  status?: string;
  moderation_comment?: string;
  moderated_at?: string;
  moderated_by?: string;
  capacity?: number;
  latitude?: number;
  longitude?: number;
  working_hours?: string;
  /** Владелец / manager / staff — из GET /owner/venues (CRM). */
  management_access?: string;
  /** Только в кабинете владельца и админке (не в публичном каталоге) */
  legal_entity_name?: string;
  inn?: string;
  ogrn?: string;
  public_listing_url?: string;
  verification_note?: string;
  /** Объект ссылок (ответ API после парсинга JSON). */
  social_links?: Partial<VenueSocialLinks> | Record<string, string>;
  created_at: string;
}

/** Ручная занятость слота (вне агрегатора), кабинет владельца */
export interface ManualSlotBlock {
  id: string;
  venue_id: string;
  date: string;
  time_from: string;
  time_to: string;
  note: string;
}

/** Участник CRM по залу (ответ /owner/venues/.../staff). */
export interface VenueStaffRow {
  user_id: string;
  role: string;
  invited_by: string;
  created_at: string;
  /** Имя из user-service (если есть). */
  user_name?: string;
  user_email?: string;
  inviter_name?: string;
  inviter_email?: string;
  /** true, если пригласивший — текущий пользователь (совпадает с invited_by). */
  inviter_is_you?: boolean;
}

/** Задача CRM по заведению. */
export interface VenueCrmTask {
  id: string;
  venue_id: string;
  title: string;
  body: string;
  status: string;
  created_by: string;
  created_at: string;
  updated_at: string;
  booking_id?: string;
  assignee_user_id?: string;
}

/** Внутренняя заметка по брони (не для гостя). */
export interface BookingStaffNote {
  id: string;
  booking_id: string;
  venue_id: string;
  author_user_id: string;
  body: string;
  created_at: string;
}

export interface Booking {
  id: string;
  user_id?: string;
  user_name?: string;
  venue_id: string;
  venue_name: string;
  service_id?: string;
  /** Несколько пакетов (ответ API после бронирования) */
  package_service_ids?: string[];
  hall_ids?: string[];
  date: string;
  time_from: string;
  time_to: string;
  time: string;
  guests: number;
  comment?: string;
  status: "pending" | "payment_pending" | "confirmed" | "completed" | "cancelled";
  total_price: number;
  /** Редирект на оплату после создания брони */
  payment_url?: string;
  created_at: string;
}

export interface Review {
  id: string;
  venue_id: string;
  master_id?: string;
  user_name: string;
  rating: number;
  text: string;
  created_at: string;
  verified: boolean;
  is_anonymous?: boolean;
}

export interface AuthResponse {
  access_token: string;
  refresh_token: string;
  user_id: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface RegisterRequest {
  name: string;
  email: string;
  phone?: string;
  password: string;
  role: "user" | "venue_owner" | "master";
}

export interface CreateBookingRequest {
  venue_id: string;
  date: string;
  time_from: string;
  time_to?: string;
  /** Услуга из карточки заведения (фиксированная цена) */
  service_id?: string;
  /** Несколько пакетов; если передано — имеет приоритет над service_id */
  service_ids?: string[];
  /** Выбранные залы (почасовая ставка — max с ценой заведения) */
  hall_ids?: string[];
  guests: number;
  comment?: string;
}

export interface CreateVenueRequest {
  name: string;
  type: "banya" | "sauna" | "hammam";
  address: string;
  city: string;
  description: string;
  phone: string;
  /** Оставлено для совместимости; при передаче `halls` пересчитывается с бэкенда */
  price_from: number;
  /** Вместимость основного зала при создании (если не передаёте `halls`) */
  capacity: number;
  amenities: string[];
  services: VenueServiceApiItem[];
  /** Как в ЕГРЮЛ / ЕГРИП — для сверки с nalog.ru / egrul.nalog.ru */
  legal_entity_name: string;
  inn: string;
  ogrn: string;
  /** Ссылка на карточку в Яндекс.Картах, 2ГИС и т.п. */
  public_listing_url: string;
  verification_note?: string;
  social_links?: VenueSocialLinks;
  /** При создании: залы (пересчитывают price_from/capacity/amenities на карточке) */
  halls?: VenueHallApiItem[];
  /** true — черновик (не в очереди модерации), пока владелец не отправит на проверку */
  start_as_draft?: boolean;
}

/** Состояние формы «новое заведение» (услуги — редактируемые строки с `key`). */
export type CreateVenueFormState = Omit<CreateVenueRequest, "services"> & {
  services: VenueServiceFormLine[];
};

/** PATCH /api/v1/venues/:id — поля карточки, доступные владельцу (тип не меняется). */
export interface UpdateVenueRequest {
  name: string;
  description: string;
  address: string;
  city: string;
  phone: string;
  latitude: number;
  longitude: number;
  working_hours: string;
  /** Залы: цена и удобства задаются по каждому залу; на карточке агрегируются автоматически. */
  halls: VenueHallFormLine[];
  /** Полный список услуг; поле `key` в JSON игнорируется бэкендом. */
  services: VenueServiceFormLine[];
  legal_entity_name: string;
  inn: string;
  ogrn: string;
  public_listing_url: string;
  verification_note?: string;
  social_links: VenueSocialLinks;
}

/** Тело PATCH /api/v1/venues/:id (услуги без поля `key`). */
export type VenueUpdatePayload = Omit<UpdateVenueRequest, "services" | "halls"> & {
  services: VenueServiceApiItem[];
  halls: VenueHallApiItem[];
};

export interface CreateReviewRequest {
  rating: number;
  text: string;
  is_anonymous?: boolean;
}

/** Длительность услуги из ответа API (поле duration_min или duration_minutes) */
export function venueServiceDurationMinutes(s: VenueService): number {
  return s.duration_min ?? s.duration_minutes ?? 0;
}

export const VENUE_TYPE_LABELS: Record<string, string> = {
  banya: "Баня",
  sauna: "Сауна",
  hammam: "Хаммам",
};

/** Статус карточки заведения в кабинете / модерации */
export const VENUE_CARD_STATUS_LABELS: Record<
  string,
  { label: string; description: string }
> = {
  draft: {
    label: "Черновик",
    description:
      "Заполните карточку и нажмите «Отправить на проверку» — до этого модератор её не видит.",
  },
  pending_review: {
    label: "На проверке",
    description:
      "Карточка на модерации. В каталоге не отображается, пока модератор не одобрит.",
  },
  active: {
    label: "Опубликовано",
    description: "Карточка видна в каталоге.",
  },
  rejected: {
    label: "Отклонено",
    description:
      "Карточка не в каталоге. Исправьте замечания и сохраните — отправится на повторную проверку.",
  },
  suspended: {
    label: "Приостановлено",
    description:
      "Публикация снята администратором. Свяжитесь с поддержкой, если непонятна причина.",
  },
};

export const BOOKING_STATUS_LABELS: Record<string, string> = {
  pending: "Ожидает",
  payment_pending: "Ожидает оплаты",
  confirmed: "Подтверждено",
  completed: "Завершено",
  cancelled: "Отменено",
};

/** Круг на карте: «сюда не выезжаю», даже внутри общей зоны выезда. */
export interface MasterTravelExcludeZone {
  id: string;
  latitude: number;
  longitude: number;
  radius_km: number;
  label: string;
}

/** Пар-мастер: услуга в профиле */
export interface MasterServiceItem {
  id: string;
  name: string;
  description: string;
  duration_min: number;
  price: number;
  sort_order: number;
}

/** Фото профиля мастера */
export interface MasterPhoto {
  id: string;
  url: string;
  sort_order: number;
  is_cover: boolean;
}

/** Как мастер принимает выплаты с платформы (ИП / ООО / физлицо / самозанятость). */
export const PAYOUT_LEGAL_FORM_LABELS: Record<string, string> = {
  ip: "Индивидуальный предприниматель (ИП)",
  ooo: "Общество с ограниченной ответственностью (ООО)",
  individual: "Физическое лицо",
  self_employed: "Самозанятость",
};

export interface MasterProfile {
  id: string;
  user_id: string;
  slug: string;
  display_name: string;
  bio: string;
  phone: string;
  city: string;
  /** ip | ooo | individual | self_employed — не отдаётся в публичном API каталога */
  payout_legal_form?: string;
  payout_legal_name?: string;
  payout_inn?: string;
  payout_kpp?: string;
  payout_ogrn?: string;
  payout_ogrnip?: string;
  payout_bank_name?: string;
  payout_bik?: string;
  payout_settlement_account?: string;
  payout_correspondent_account?: string;
  payout_verification_status?: "unverified" | "pending" | "verified" | "rejected" | string;
  payout_ready?: boolean;
  work_format: string;
  /** Макс. расстояние от метки на карте до клиента, км (по прямой на карте; API: travel_radius_km). */
  travel_radius_km: number;
  /** Координаты метки на карте (точка отсчёта километража). */
  travel_base_latitude?: number;
  travel_base_longitude?: number;
  travel_exclude_zones?: MasterTravelExcludeZone[];
  experience_years: number;
  specializations: string[];
  hourly_rate: number;
  availability_json: string;
  status: string;
  moderation_comment: string;
  moderated_by?: string;
  moderated_at?: string;
  services: MasterServiceItem[];
  photos?: MasterPhoto[];
  created_at?: string;
  updated_at?: string;
}

export interface MasterBooking {
  id: string;
  master_id: string;
  client_user_id: string;
  master_service_id?: string;
  date: string;
  time_from: string;
  time_to: string;
  comment: string;
  status: string;
  payment_id?: string;
  payment_url?: string;
  total_price?: number;
  created_at: string;
}

export const MASTER_PROFILE_STATUS_LABELS: Record<
  string,
  { label: string; description: string }
> = {
  draft: {
    label: "Черновик",
    description:
      "Заполните профиль и отправьте на модерацию — до этого клиенты вас не видят.",
  },
  pending_review: {
    label: "На модерации",
    description: "Профиль проверяется. Редактирование доступно; при необходимости модератор вернёт с замечаниями.",
  },
  needs_revision: {
    label: "Нужны правки",
    description: "Исправьте замечания ниже и снова отправьте профиль на проверку.",
  },
  active: {
    label: "В каталоге",
    description: "Профиль опубликован. После любых правок снова потребуется модерация.",
  },
  rejected: {
    label: "Отклонён",
    description: "Публикация отклонена. Можно изменить профиль и отправить снова.",
  },
  suspended: {
    label: "Снят с публикации",
    description: "Профиль временно скрыт модератором.",
  },
};

export interface ChatParticipantRead {
  user_id: string;
  /** Furthest message id this user has acknowledged (MarkRead / open thread). */
  last_read_message_id?: string;
}

export interface ChatThread {
  id: string;
  kind: "venue_booking" | "master_booking" | string;
  ref_id: string;
  participant_user_ids: string[];
  last_message_id?: string;
  last_message_at?: string;
  unread_count: number;
  participant_reads?: ChatParticipantRead[];
  /** Имя собеседника в списке чатов (клиент для владельца заведения, заведение для гостя и т.д.). */
  peer_display_name?: string;
}

export interface ChatMessage {
  id: string;
  thread_id: string;
  author_user_id: string;
  text: string;
  client_msg_id?: string;
  created_at?: string;
}

export interface SupportContactRequest {
  topic: string;
  message: string;
  email?: string;
  booking_id?: string;
  payment_id?: string;
  source_page?: string;
}

export interface SupportContactResponse {
  request_id: string;
  ticket_number?: string;
  status: string;
}

/** Ответ модератора пользователю по ранее созданному обращению (email через SMTP). */
export interface SupportTicketReplyRequest {
  ticket_number?: string;
  request_id?: string;
  user_email: string;
  reply: string;
}

export interface SupportTicketReplyResponse {
  status: string;
}

/** Запись в очереди обращений (модератор, GET /admin/support/tickets). */
export interface SupportTicketAdmin {
  request_id: string;
  ticket_number: string;
  topic: string;
  message: string;
  user_email: string;
  user_id: string;
  role: string;
  booking_id: string;
  payment_id: string;
  source_page: string;
  created_at: string;
  replied_at: string | null;
  replied_by: string;
  /** Доставка уведомления модератору (email/webhook): pending | ok | failed */
  notify_status?: string;
}

export interface SupportTicketsListResponse {
  tickets: SupportTicketAdmin[];
  total: number;
}

// ── Кабинет выплат партнёра (venue / master) ───────────────────────────────

/** Способ выплаты партнёру. Реквизиты приходят только в маскированном виде. */
export interface PayoutMethod {
  id: string;
  partner_type: "venue" | "master";
  partner_id: string;
  kind: "card" | "bank_account" | "sbp";
  provider_name?: string;
  // card
  card_last4?: string;
  card_brand?: string;
  // bank_account
  bank_bic?: string;
  bank_account_masked?: string;
  bank_name?: string;
  recipient_name?: string;
  // sbp
  sbp_phone_masked?: string;
  sbp_bank_id?: string;
  created_at?: string;
  updated_at?: string;
}

/** Реквизиты для сохранения способа выплаты (PUT payout-method). */
export interface SetPayoutMethodInput {
  kind: "bank_account" | "sbp";
  // bank_account
  bank_bic?: string;
  bank_account?: string;
  bank_name?: string;
  recipient_inn?: string;
  recipient_kpp?: string;
  recipient_name?: string;
  // sbp
  sbp_phone?: string;
  sbp_bank_id?: string;
}

/** Баланс партнёра: total = весь долг, available = к выплате, held = на удержании. */
export interface PartnerBalance {
  partner_type: string;
  partner_id: string;
  total_kopecks: number;
  available_kopecks: number;
  held_kopecks: number;
  last_entry_at?: string;
}

/** Строка журнала начислений (accrual | reversal | payout | adjustment). */
export interface PartnerLedgerEntry {
  id: number;
  partner_type: string;
  partner_id: string;
  entry_type: "accrual" | "reversal" | "payout" | "adjustment";
  amount_kopecks: number;
  payment_id?: string;
  payout_id?: string;
  reverses_entry_id?: number;
  reason?: string;
  available_at?: string;
  created_at?: string;
}

/** Исходящая выплата партнёру. */
export interface PartnerPayout {
  id: string;
  partner_type: string;
  partner_id: string;
  amount_kopecks: number;
  currency: string;
  status: "pending" | "processing" | "succeeded" | "failed";
  method_kind: string;
  method_display: string;
  provider_name?: string;
  provider_payout_id?: string;
  failure_reason?: string;
  created_at?: string;
  completed_at?: string;
}

export const PAYOUT_STATUS_LABELS: Record<string, string> = {
  pending: "В очереди",
  processing: "Обрабатывается",
  succeeded: "Выплачено",
  failed: "Ошибка",
};

export const LEDGER_ENTRY_LABELS: Record<string, string> = {
  accrual: "Начисление",
  reversal: "Возврат",
  payout: "Выплата",
  adjustment: "Корректировка",
};
