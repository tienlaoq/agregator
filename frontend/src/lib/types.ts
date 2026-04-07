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
}

export interface Booking {
  id: string;
  venue_id: string;
  venue_name: string;
  date: string;
  time: string;
  guests: number;
  status: "pending" | "confirmed" | "completed" | "cancelled";
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
  phone: string;
  password: string;
  role: "visitor" | "owner";
}

export interface CreateBookingRequest {
  venue_id: string;
  date: string;
  time: string;
  guests: number;
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
  confirmed: "Подтверждено",
  completed: "Завершено",
  cancelled: "Отменено",
};
