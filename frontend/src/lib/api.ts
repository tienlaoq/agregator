import type {
  AuthResponse,
  Booking,
  CreateBookingRequest,
  CreateReviewRequest,
  BookingStaffNote,
  CreateVenueRequest,
  LoginRequest,
  ManualSlotBlock,
  RegisterRequest,
  Review,
  VenueCrmTask,
  VenueStaffRow,
  VenueUpdatePayload,
  User,
  Venue,
  MasterProfile,
  MasterBooking,
} from "./types";
import { userMessageForGatewayError } from "./api-user-messages";
import { packCitiesForQuery } from "./cities-http";

/** Base URL браузера (и публичные ссылки на медиа). */
const PUBLIC_API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

/** URL api-gateway для fetch: в контейнере фронта — внутренний хост compose. */
function apiUrlForFetch(): string {
  if (typeof window !== "undefined") {
    return PUBLIC_API_URL;
  }
  return process.env.INTERNAL_API_URL || PUBLIC_API_URL;
}

export class ApiError extends Error {
  /** Machine code from JSON `code` when gateway returned a catalog error. */
  public readonly code?: string;

  constructor(
    public status: number,
    /** Raw response body (for tests/logs; do not show in UI). */
    message: string,
    code?: string,
  ) {
    super(message);
    this.name = "ApiError";
    this.code = code;
  }
}

function parseGatewayErrorCode(text: string): string | undefined {
  const t = text.trim();
  if (!t.startsWith("{")) return undefined;
  try {
    const j = JSON.parse(t) as { code?: string };
    return typeof j.code === "string" && j.code ? j.code : undefined;
  } catch {
    return undefined;
  }
}

let refreshPromise: Promise<boolean> | null = null;

async function tryRefreshToken(): Promise<boolean> {
  const refreshToken =
    typeof window !== "undefined" ? localStorage.getItem("refresh_token") : null;
  if (!refreshToken) return false;

  try {
    const res = await fetch(`${apiUrlForFetch()}/api/v1/auth/refresh`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: refreshToken }),
    });
    if (!res.ok) return false;

    const data = await res.json();
    localStorage.setItem("token", data.access_token);
    localStorage.setItem("refresh_token", data.refresh_token);

    const { useAuthStore } = await import("@/store/auth");
    useAuthStore.getState().setTokens(data.access_token, data.refresh_token);
    return true;
  } catch {
    return false;
  }
}

async function fetchAPI<T>(path: string, options?: RequestInit): Promise<T> {
  const token =
    typeof window !== "undefined" ? localStorage.getItem("token") : null;

  const isFormData =
    typeof FormData !== "undefined" && options?.body instanceof FormData;
  const headers: Record<string, string> = {
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
  };
  if (!isFormData) {
    headers["Content-Type"] = "application/json";
  }

  const method = (options?.method ?? "GET").toUpperCase();
  const defaultCache: RequestCache | undefined =
    method === "GET" || method === "HEAD" ? "no-store" : undefined;

  const res = await fetch(`${apiUrlForFetch()}${path}`, {
    ...options,
    cache: options?.cache ?? defaultCache,
    headers: {
      ...headers,
      ...(options?.headers as Record<string, string>),
    },
  });

  if (res.status === 401 && typeof window !== "undefined") {
    if (!refreshPromise) {
      refreshPromise = tryRefreshToken().finally(() => {
        refreshPromise = null;
      });
    }
    const refreshed = await refreshPromise;
    if (refreshed) {
      const newToken = localStorage.getItem("token");
      const retryHeaders: Record<string, string> = {
        ...(options?.headers as Record<string, string>),
        ...(newToken ? { Authorization: `Bearer ${newToken}` } : {}),
      };
      if (!isFormData) {
        retryHeaders["Content-Type"] = "application/json";
      }
      const retryRes = await fetch(`${apiUrlForFetch()}${path}`, {
        ...options,
        cache: options?.cache ?? defaultCache,
        headers: retryHeaders,
      });

      if (!retryRes.ok) {
        const text = await retryRes.text().catch(() => "");
        throw new ApiError(retryRes.status, text, parseGatewayErrorCode(text));
      }

      if (retryRes.status === 204) return undefined as T;
      return retryRes.json();
    }

    const { useAuthStore } = await import("@/store/auth");
    useAuthStore.getState().logout();
    if (typeof window !== "undefined") {
      window.location.href = "/auth/login";
    }
  }

  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new ApiError(res.status, text, parseGatewayErrorCode(text));
  }

  if (res.status === 204) return undefined as T;
  return res.json();
}

export async function login(data: LoginRequest): Promise<AuthResponse> {
  return fetchAPI<AuthResponse>("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function register(data: RegisterRequest): Promise<AuthResponse> {
  return fetchAPI<AuthResponse>("/api/v1/auth/register", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

/** Собирает пользователя для стора без лишнего GET /users/me сразу после регистрации. */
export function userFromRegisterResponse(
  res: AuthResponse,
  fields: Pick<RegisterRequest, "name" | "email" | "role"> & { phone?: string },
): User {
  return {
    id: res.user_id,
    email: fields.email,
    name: fields.name,
    phone: fields.phone ?? "",
    role: fields.role,
  };
}

interface PaginatedVenues {
  venues: Venue[];
  page: number;
  page_size: number;
  total: number;
}

export async function getVenues(
  params?: {
    page?: number;
    page_size?: number;
    type?: string;
    sort_by?: string;
  },
  init?: RequestInit,
): Promise<PaginatedVenues> {
  const search = new URLSearchParams();
  if (params) {
    Object.entries(params).forEach(([k, v]) => {
      if (v !== undefined && v !== "") search.set(k, String(v));
    });
  }
  const qs = search.toString();
  return fetchAPI<PaginatedVenues>(`/api/v1/venues${qs ? `?${qs}` : ""}`, init);
}

function normalizeCityParamsSafe(city?: string | string[]): string[] {
  const raw = Array.isArray(city) ? city : city != null && String(city).trim() ? [String(city).trim()] : [];
  const seen = new Set<string>();
  const out: string[] = [];
  for (const s of raw) {
    const t = String(s).trim();
    if (!t) continue;
    const k = t.toLowerCase();
    if (seen.has(k)) continue;
    seen.add(k);
    out.push(t);
  }
  return out;
}

export async function searchVenues(
  params: {
    q?: string;
    /** Один город или несколько (в URL: `city=` или упакованный `cities=`) */
    city?: string | string[];
    type?: string;
    price_min?: number;
    price_max?: number;
    rating_min?: number;
    page?: number;
    page_size?: number;
  },
  init?: RequestInit,
): Promise<PaginatedVenues> {
  const q = (params.q ?? "").trim();
  const cities = normalizeCityParamsSafe(params.city);
  // Один город: дублируем в q для старых билдов. 2+ — один параметр cities= (иначе Next схлопывает city=&city=).
  const qOut = q || (cities.length === 1 ? cities[0] : "");

  const search = new URLSearchParams();
  if (qOut) search.set("q", qOut);
  if (cities.length === 1) {
    search.set("city", cities[0]);
  } else if (cities.length > 1) {
    const packed = packCitiesForQuery(cities);
    if (packed) search.set("cities", packed);
  }
  if (params.type) search.set("type", params.type);
  if (params.price_min != null && params.price_min !== 0) {
    search.set("price_min", String(params.price_min));
  }
  if (params.price_max != null && params.price_max !== 0) {
    search.set("price_max", String(params.price_max));
  }
  if (params.rating_min != null && params.rating_min !== 0) {
    search.set("rating_min", String(params.rating_min));
  }
  if (params.page != null && params.page !== 0) {
    search.set("page", String(params.page));
  }
  if (params.page_size != null && params.page_size !== 0) {
    search.set("page_size", String(params.page_size));
  }
  const qs = search.toString();
  return fetchAPI<PaginatedVenues>(`/api/v1/venues/search${qs ? `?${qs}` : ""}`, init);
}

export async function getVenueBySlug(slug: string): Promise<Venue> {
  return fetchAPI<Venue>(`/api/v1/venues/${slug}`);
}

export async function getVenueAvailability(
  slug: string,
  date: string,
  durationMin?: number,
): Promise<{ date: string; available_slots: string[] }> {
  const q = new URLSearchParams({ date });
  if (
    durationMin != null &&
    durationMin >= 30 &&
    durationMin <= 720
  ) {
    q.set("duration_min", String(durationMin));
  }
  return fetchAPI<{ date: string; available_slots: string[] }>(
    `/api/v1/venues/${encodeURIComponent(slug)}/availability?${q.toString()}`,
  );
}

export async function getVenueReviews(venueId: string): Promise<Review[]> {
  const data = await fetchAPI<{ reviews: Review[]; total: number }>(`/api/v1/venues/${venueId}/reviews`);
  return data.reviews ?? [];
}

export async function createReview(
  venueId: string,
  data: CreateReviewRequest,
): Promise<Review> {
  return fetchAPI<Review>(`/api/v1/venues/${venueId}/reviews`, {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function createBooking(
  data: CreateBookingRequest,
): Promise<Booking> {
  return fetchAPI<Booking>("/api/v1/bookings", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function getMyBookings(): Promise<Booking[]> {
  const data = await fetchAPI<{ bookings: Booking[]; total: number }>("/api/v1/bookings/my");
  return data.bookings ?? [];
}

export async function getOwnerVenueBookings(
  venueId: string,
  params?: {
    status?: string;
    date?: string;
    page?: number;
    page_size?: number;
  },
): Promise<{ bookings: Booking[]; total: number }> {
  const search = new URLSearchParams();
  if (params?.status) search.set("status", params.status);
  if (params?.date) search.set("date", params.date);
  if (params?.page != null) search.set("page", String(params.page));
  if (params?.page_size != null) search.set("page_size", String(params.page_size));
  const qs = search.toString();
  return fetchAPI<{ bookings: Booking[]; total: number }>(
    `/api/v1/owner/venues/${venueId}/bookings${qs ? `?${qs}` : ""}`,
  );
}

export async function listOwnerSlotBlocks(
  venueId: string,
  dateFrom: string,
  dateTo: string,
): Promise<{ blocks: ManualSlotBlock[] }> {
  const q = new URLSearchParams({ date_from: dateFrom, date_to: dateTo });
  return fetchAPI<{ blocks: ManualSlotBlock[] }>(
    `/api/v1/owner/venues/${encodeURIComponent(venueId)}/slot-blocks?${q.toString()}`,
  );
}

export async function createOwnerSlotBlock(
  venueId: string,
  body: { date: string; time_from: string; time_to: string; note?: string },
): Promise<{ block: ManualSlotBlock }> {
  return fetchAPI<{ block: ManualSlotBlock }>(
    `/api/v1/owner/venues/${encodeURIComponent(venueId)}/slot-blocks`,
    { method: "POST", body: JSON.stringify(body) },
  );
}

export async function deleteOwnerSlotBlock(
  venueId: string,
  blockId: string,
): Promise<void> {
  return fetchAPI<void>(
    `/api/v1/owner/venues/${encodeURIComponent(venueId)}/slot-blocks/${encodeURIComponent(blockId)}`,
    { method: "DELETE" },
  );
}

export async function cancelBooking(id: string): Promise<void> {
  return fetchAPI<void>(`/api/v1/bookings/${id}/cancel`, {
    method: "POST",
  });
}

export async function getProfile(): Promise<User> {
  return fetchAPI<User>("/api/v1/users/me");
}

export async function updateProfile(
  data: Partial<Pick<User, "name" | "phone">>,
): Promise<User> {
  return fetchAPI<User>("/api/v1/users/me", {
    method: "PATCH",
    body: JSON.stringify(data),
  });
}

export async function getOwnerVenues(): Promise<Venue[]> {
  const data = await fetchAPI<{ venues: Venue[]; total: number }>("/api/v1/owner/venues");
  return data.venues ?? [];
}

export async function listVenueStaff(venueId: string): Promise<VenueStaffRow[]> {
  const data = await fetchAPI<{ staff: VenueStaffRow[] }>(
    `/api/v1/owner/venues/${encodeURIComponent(venueId)}/staff`,
  );
  return data.staff ?? [];
}

export async function inviteVenueStaffByEmail(
  venueId: string,
  body: { email: string; role: string },
): Promise<{ user_id: string; email: string; role: string }> {
  return fetchAPI(`/api/v1/owner/venues/${encodeURIComponent(venueId)}/staff`, {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export async function removeVenueStaff(
  venueId: string,
  userId: string,
): Promise<void> {
  return fetchAPI<void>(
    `/api/v1/owner/venues/${encodeURIComponent(venueId)}/staff/${encodeURIComponent(userId)}`,
    { method: "DELETE" },
  );
}

export async function listVenueCrmTasks(
  venueId: string,
  params?: { status?: string },
): Promise<VenueCrmTask[]> {
  const q = new URLSearchParams();
  if (params?.status) q.set("status", params.status);
  const qs = q.toString();
  const data = await fetchAPI<{ tasks: VenueCrmTask[] }>(
    `/api/v1/owner/venues/${encodeURIComponent(venueId)}/crm/tasks${qs ? `?${qs}` : ""}`,
  );
  return data.tasks ?? [];
}

export async function createVenueCrmTask(
  venueId: string,
  body: {
    title: string;
    body: string;
    booking_id?: string;
    assignee_user_id?: string;
  },
): Promise<{ task: VenueCrmTask }> {
  return fetchAPI(`/api/v1/owner/venues/${encodeURIComponent(venueId)}/crm/tasks`, {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export async function completeVenueCrmTask(
  venueId: string,
  taskId: string,
): Promise<void> {
  return fetchAPI<void>(
    `/api/v1/owner/venues/${encodeURIComponent(venueId)}/crm/tasks/${encodeURIComponent(taskId)}/complete`,
    { method: "POST" },
  );
}

export async function listBookingStaffNotes(
  venueId: string,
  bookingId: string,
): Promise<BookingStaffNote[]> {
  const data = await fetchAPI<{ notes: BookingStaffNote[] }>(
    `/api/v1/owner/venues/${encodeURIComponent(venueId)}/bookings/${encodeURIComponent(bookingId)}/staff-notes`,
  );
  return data.notes ?? [];
}

export async function addBookingStaffNote(
  venueId: string,
  bookingId: string,
  body: string,
): Promise<{ note: BookingStaffNote }> {
  return fetchAPI(
    `/api/v1/owner/venues/${encodeURIComponent(venueId)}/bookings/${encodeURIComponent(bookingId)}/staff-notes`,
    { method: "POST", body: JSON.stringify({ body }) },
  );
}

export async function createVenue(data: CreateVenueRequest): Promise<Venue> {
  return fetchAPI<Venue>("/api/v1/venues", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function updateVenue(
  venueId: string,
  data: VenueUpdatePayload,
): Promise<Venue> {
  return fetchAPI<Venue>(`/api/v1/venues/${venueId}`, {
    method: "PATCH",
    body: JSON.stringify(data),
  });
}

export async function submitVenueForReview(venueId: string): Promise<Venue> {
  return fetchAPI<Venue>(`/api/v1/venues/${venueId}/submit-for-review`, {
    method: "POST",
  });
}

/** Absolute URL for venue photo path from API (e.g. /api/v1/uploads/...) or full URL. */
export function venueMediaUrl(pathOrUrl: string | undefined): string {
  if (!pathOrUrl) return "";
  if (pathOrUrl.startsWith("http://") || pathOrUrl.startsWith("https://")) {
    return pathOrUrl;
  }
  const base = PUBLIC_API_URL.replace(/\/$/, "");
  const p = pathOrUrl.startsWith("/") ? pathOrUrl : `/${pathOrUrl}`;
  return `${base}${p}`;
}

/** Превью для каталога/публичной карточки: image_url или обложка/первое фото. */
export function venueCardImageSrc(v: {
  image_url?: string;
  photos?: { url: string; is_cover?: boolean }[];
}): string | undefined {
  const raw =
    (v.image_url && v.image_url.trim()) ||
    v.photos?.find((p) => p.is_cover)?.url?.trim() ||
    v.photos?.[0]?.url?.trim();
  if (!raw) return undefined;
  return venueMediaUrl(raw);
}

export async function uploadVenuePhoto(
  venueId: string,
  file: File,
): Promise<Venue> {
  const form = new FormData();
  form.append("photo", file);
  return fetchAPI<Venue>(`/api/v1/venues/${venueId}/photos`, {
    method: "POST",
    body: form,
  });
}

export async function deleteVenuePhoto(
  venueId: string,
  photoId: string,
): Promise<Venue> {
  return fetchAPI<Venue>(`/api/v1/venues/${venueId}/photos/${photoId}`, {
    method: "DELETE",
  });
}

export async function setVenueCoverPhoto(
  venueId: string,
  photoId: string,
): Promise<Venue> {
  return fetchAPI<Venue>(`/api/v1/venues/${venueId}/photos/${photoId}/cover`, {
    method: "POST",
  });
}

export async function uploadVenueHallPhoto(
  venueId: string,
  hallId: string,
  file: File,
): Promise<Venue> {
  const form = new FormData();
  form.append("photo", file);
  return fetchAPI<Venue>(
    `/api/v1/venues/${venueId}/halls/${hallId}/photos`,
    {
      method: "POST",
      body: form,
    },
  );
}

export async function deleteVenueHallPhoto(
  venueId: string,
  hallId: string,
  photoId: string,
): Promise<Venue> {
  return fetchAPI<Venue>(
    `/api/v1/venues/${venueId}/halls/${hallId}/photos/${photoId}`,
    { method: "DELETE" },
  );
}

export async function setVenueHallCoverPhoto(
  venueId: string,
  hallId: string,
  photoId: string,
): Promise<Venue> {
  return fetchAPI<Venue>(
    `/api/v1/venues/${venueId}/halls/${hallId}/photos/${photoId}/cover`,
    { method: "POST" },
  );
}

// Masters (public)
export async function listPublicMasters(params?: {
  city?: string;
  limit?: number;
  offset?: number;
}): Promise<{ masters: MasterProfile[]; total: number }> {
  const search = new URLSearchParams();
  if (params?.city) search.set("city", params.city);
  if (params?.limit != null) search.set("limit", String(params.limit));
  if (params?.offset != null) search.set("offset", String(params.offset));
  const qs = search.toString();
  return fetchAPI<{ masters: MasterProfile[]; total: number }>(
    `/api/v1/masters${qs ? `?${qs}` : ""}`,
  );
}

export async function getPublicMaster(slug: string): Promise<MasterProfile> {
  return fetchAPI<MasterProfile>(`/api/v1/masters/${encodeURIComponent(slug)}`);
}

// Master cabinet (role master)
export async function createMasterProfile(
  display_name: string,
): Promise<MasterProfile> {
  return fetchAPI<MasterProfile>("/api/v1/owner/master/profile", {
    method: "POST",
    body: JSON.stringify({ display_name }),
  });
}

export async function getMyMasterProfile(): Promise<MasterProfile | null> {
  const r = await fetchAPI<{ profile: MasterProfile | null }>(
    "/api/v1/owner/master/profile",
  );
  return r.profile ?? null;
}

export async function patchMyMasterProfile(
  body: Record<string, unknown>,
): Promise<MasterProfile> {
  return fetchAPI<MasterProfile>("/api/v1/owner/master/profile", {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export async function submitMasterForReview(
  body?: Record<string, unknown>,
): Promise<MasterProfile> {
  return fetchAPI<MasterProfile>(
    "/api/v1/owner/master/profile/submit-for-review",
    { method: "POST", body: JSON.stringify(body ?? {}) },
  );
}

export async function uploadMasterPhoto(file: File): Promise<MasterProfile> {
  const form = new FormData();
  form.append("photo", file);
  return fetchAPI<MasterProfile>("/api/v1/owner/master/profile/photos", {
    method: "POST",
    body: form,
  });
}

export async function deleteMasterPhoto(
  photoId: string,
): Promise<{ deleted_url: string }> {
  return fetchAPI<{ deleted_url: string }>(
    `/api/v1/owner/master/profile/photos/${encodeURIComponent(photoId)}`,
    { method: "DELETE" },
  );
}

export async function setMasterCoverPhoto(
  photoId: string,
): Promise<MasterProfile> {
  return fetchAPI<MasterProfile>(
    `/api/v1/owner/master/profile/photos/${encodeURIComponent(photoId)}/cover`,
    { method: "POST" },
  );
}

/** Превью мастера: обложка или первое фото. */
export function masterCardImageSrc(m: {
  photos?: { url: string; is_cover?: boolean }[];
}): string | undefined {
  const raw =
    m.photos?.find((p) => p.is_cover)?.url?.trim() ||
    m.photos?.[0]?.url?.trim();
  if (!raw) return undefined;
  return venueMediaUrl(raw);
}

/**
 * Цена для строки «от N ₽ / час» в каталоге и карточке: минимум среди цен услуг (коп.),
 * иначе базовая ставка профиля. null — не показывать блок.
 */
export function masterCardFromPriceKopecks(m: MasterProfile): number | null {
  const services = m.services ?? [];
  let min = Infinity;
  for (const s of services) {
    const p = s.price;
    if (typeof p === "number" && p > 0 && p < min) min = p;
  }
  if (min !== Infinity) return min;
  if (typeof m.hourly_rate === "number" && m.hourly_rate > 0) return m.hourly_rate;
  return null;
}

/** Человекочитаемая строка цены для карточки мастера или null. */
export function masterCardPriceLabel(m: MasterProfile): string | null {
  const k = masterCardFromPriceKopecks(m);
  if (k == null) return null;
  const rub = new Intl.NumberFormat("ru-RU", {
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  }).format(k / 100);
  return `от ${rub} ₽ / час`;
}

export async function listMyMasterBookings(params?: {
  status?: string;
}): Promise<{ bookings: MasterBooking[] }> {
  const search = new URLSearchParams();
  if (params?.status) search.set("status", params.status);
  const qs = search.toString();
  return fetchAPI<{ bookings: MasterBooking[] }>(
    `/api/v1/owner/master/bookings${qs ? `?${qs}` : ""}`,
  );
}

export async function createMasterBooking(
  slug: string,
  data: {
    master_service_id?: string;
    date: string;
    time_from: string;
    time_to: string;
    comment?: string;
  },
): Promise<MasterBooking> {
  return fetchAPI<MasterBooking>(
    `/api/v1/masters/${encodeURIComponent(slug)}/bookings`,
    {
      method: "POST",
      body: JSON.stringify(data),
    },
  );
}

// Admin API
export async function getAdminVenues(params?: {
  status?: string;
  page?: number;
  page_size?: number;
  /** Подстрока в названии (серверный ILIKE), см. GET /admin/venues?q= */
  q?: string;
}): Promise<PaginatedVenues> {
  const search = new URLSearchParams();
  if (params) {
    Object.entries(params).forEach(([k, v]) => {
      if (v !== undefined && v !== "") search.set(k, String(v));
    });
  }
  const qs = search.toString();
  return fetchAPI<PaginatedVenues>(`/api/v1/admin/venues${qs ? `?${qs}` : ""}`);
}

export async function moderateVenue(
  venueId: string,
  action: "approve" | "reject" | "suspend" | "resume",
  comment?: string,
): Promise<Venue> {
  return fetchAPI<Venue>(`/api/v1/admin/venues/${venueId}/moderate`, {
    method: "POST",
    body: JSON.stringify({ action, comment: comment || "" }),
  });
}

export async function getAdminMasters(params?: {
  status?: string;
  limit?: number;
  offset?: number;
}): Promise<{ masters: MasterProfile[]; total: number }> {
  const search = new URLSearchParams();
  if (params?.status) search.set("status", params.status);
  if (params?.limit != null) search.set("limit", String(params.limit));
  if (params?.offset != null) search.set("offset", String(params.offset));
  const qs = search.toString();
  return fetchAPI<{ masters: MasterProfile[]; total: number }>(
    `/api/v1/admin/masters${qs ? `?${qs}` : ""}`,
  );
}

export async function moderateMaster(
  masterId: string,
  action: "approve" | "request_revision" | "reject" | "suspend",
  comment?: string,
): Promise<MasterProfile> {
  return fetchAPI<MasterProfile>(`/api/v1/admin/masters/${masterId}/moderate`, {
    method: "POST",
    body: JSON.stringify({ action, comment: comment || "" }),
  });
}

export async function getMasterModerationHistory(
  masterId: string,
  limit?: number,
): Promise<{
  entries: {
    id: string;
    master_id: string;
    old_status: string;
    new_status: string;
    comment: string;
    changed_by: string;
    created_at: string;
  }[];
}> {
  const search = new URLSearchParams();
  if (limit != null) search.set("limit", String(limit));
  const qs = search.toString();
  return fetchAPI(
    `/api/v1/admin/masters/${masterId}/moderation-history${qs ? `?${qs}` : ""}`,
  );
}

/** Short Russian text for UI; never exposes raw API/gRPC bodies. */
export function formatApiErrorMessage(e: unknown, fallback: string): string {
  if (e instanceof ApiError) {
    return userMessageForGatewayError(e.status, e.code, fallback);
  }
  if (e instanceof Error && e.message) {
    const m = e.message;
    if (/failed to fetch|load failed|networkerror|network request failed/i.test(m)) {
      return "Нет соединения с сервером. Проверьте интернет и попробуйте снова.";
    }
  }
  return fallback;
}
