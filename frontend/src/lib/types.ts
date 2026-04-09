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
  duration_minutes: number;
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
  amenities: string[];
  services: VenueService[];
  owner_id: string;
  status?: string;
  moderation_comment?: string;
  moderated_at?: string;
  moderated_by?: string;
  /** Только в кабинете владельца и админке (не в публичном каталоге) */
  legal_entity_name?: string;
  inn?: string;
  ogrn?: string;
  public_listing_url?: string;
  verification_note?: string;
  created_at: string;
}

export interface Booking {
  id: string;
  venue_id: string;
  venue_name: string;
  date: string;
  time_from: string;
  time_to: string;
  time: string;
  guests: number;
  comment?: string;
  status: "pending" | "payment_pending" | "confirmed" | "completed" | "cancelled";
  total_price: number;
  created_at: string;
}

export interface Review {
  id: string;
  venue_id: string;
  user_name: string;
  rating: number;
  text: string;
  created_at: string;
  verified: boolean;
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
  role: "user" | "venue_owner";
}

export interface CreateBookingRequest {
  venue_id: string;
  date: string;
  time_from: string;
  time_to?: string;
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
  price_from: number;
  amenities: string[];
  services: Omit<VenueService, "id">[];
  /** Как в ЕГРЮЛ / ЕГРИП — для сверки с nalog.ru / egrul.nalog.ru */
  legal_entity_name: string;
  inn: string;
  ogrn: string;
  /** Ссылка на карточку в Яндекс.Картах, 2ГИС и т.п. */
  public_listing_url: string;
  verification_note?: string;
}

export interface CreateReviewRequest {
  rating: number;
  text: string;
}

export const VENUE_TYPE_LABELS: Record<string, string> = {
  banya: "Баня",
  sauna: "Сауна",
  hammam: "Хаммам",
};

export const BOOKING_STATUS_LABELS: Record<string, string> = {
  pending: "Ожидает",
  payment_pending: "Ожидает оплаты",
  confirmed: "Подтверждено",
  completed: "Завершено",
  cancelled: "Отменено",
};
