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
  ChatThread,
  ChatMessage,
  SupportContactRequest,
  SupportContactResponse,
  SupportTicketReplyRequest,
  SupportTicketReplyResponse,
  SupportTicketsListResponse,
} from "./types";
import { userMessageForGatewayError } from "./api-user-messages";
import { packCitiesForQuery } from "./cities-http";
import { chatV2Paths } from "./chat-paths";

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
  /** Human-readable `error` from gateway JSON when present (safe subset shown in UI). */
  public readonly gatewayDetail?: string;

  constructor(
    public status: number,
    /** Raw response body (for tests/logs; do not show in UI). */
    message: string,
    code?: string,
    gatewayDetail?: string,
  ) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.gatewayDetail = gatewayDetail;
  }
}

function parseGatewayError(text: string): { code?: string; detail?: string } {
  const t = text.trim();
  if (!t.startsWith("{")) return {};
  try {
    const j = JSON.parse(t) as { code?: string; error?: string };
    const code = typeof j.code === "string" && j.code ? j.code : undefined;
    const detail =
      typeof j.error === "string" && j.error.trim() ? j.error.trim() : undefined;
    return { code, detail };
  } catch {
    return {};
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
        const g = parseGatewayError(text);
        throw new ApiError(retryRes.status, text, g.code, g.detail);
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
    const g = parseGatewayError(text);
    throw new ApiError(res.status, text, g.code, g.detail);
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

export async function requestPasswordReset(email: string): Promise<{ message: string }> {
  return fetchAPI<{ message: string }>("/api/v1/auth/forgot-password", {
    method: "POST",
    body: JSON.stringify({ email }),
  });
}

export async function completePasswordReset(
  token: string,
  newPassword: string,
): Promise<{ status: string }> {
  return fetchAPI<{ status: string }>("/api/v1/auth/reset-password", {
    method: "POST",
    body: JSON.stringify({ token, new_password: newPassword }),
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

export async function getMasterReviews(masterId: string): Promise<Review[]> {
  const data = await fetchAPI<{ reviews: Review[]; total: number }>(`/api/v1/masters/${masterId}/reviews`);
  return data.reviews ?? [];
}

export async function createMasterReview(
  masterId: string,
  data: CreateReviewRequest,
): Promise<Review> {
  return fetchAPI<Review>(`/api/v1/masters/${masterId}/reviews`, {
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

function venueStaffStr(v: unknown): string {
  return typeof v === "string" ? v.trim() : "";
}

function venueStaffPick(
  raw: Record<string, unknown>,
  snake: string,
  camel: string,
): string | undefined {
  const a = venueStaffStr(raw[snake]);
  if (a) return a;
  const b = venueStaffStr(raw[camel]);
  return b || undefined;
}

function mapVenueStaffFromApi(raw: Record<string, unknown>): VenueStaffRow {
  const user_id =
    venueStaffStr(raw.user_id) || venueStaffStr(raw.userId);
  const invited_by =
    venueStaffStr(raw.invited_by) || venueStaffStr(raw.invitedBy);
  const created_at = venueStaffStr(raw.created_at);
  return {
    user_id,
    role: venueStaffStr(raw.role),
    invited_by,
    created_at,
    user_name: venueStaffPick(raw, "user_name", "userName"),
    user_email: venueStaffPick(raw, "user_email", "userEmail"),
    inviter_name: venueStaffPick(raw, "inviter_name", "inviterName"),
    inviter_email: venueStaffPick(raw, "inviter_email", "inviterEmail"),
    inviter_is_you: Boolean(raw.inviter_is_you ?? raw.inviterIsYou),
  };
}

export async function listVenueStaff(venueId: string): Promise<VenueStaffRow[]> {
  const data = await fetchAPI<{ staff: unknown[] }>(
    `/api/v1/owner/venues/${encodeURIComponent(venueId)}/staff`,
  );
  const rows = data.staff ?? [];
  return rows.map((r) =>
    mapVenueStaffFromApi(
      r && typeof r === "object" ? (r as Record<string, unknown>) : {},
    ),
  );
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

// Masters (public) — сборка query как у searchVenues (q + city/cities, один город дублируется в q).
export async function listPublicMasters(
  params?: {
    q?: string;
    /** Один город или несколько (в URL: `city=` или упакованный `cities=`) */
    city?: string | string[];
    work_format?: string;
    price_min?: number;
    price_max?: number;
    page?: number;
    page_size?: number;
    limit?: number;
    offset?: number;
  },
  init?: RequestInit,
): Promise<{ masters: MasterProfile[]; total: number }> {
  const q = (params?.q ?? "").trim();
  const cities = normalizeCityParamsSafe(params?.city);
  const qOut = q || (cities.length === 1 ? cities[0] : "");

  const search = new URLSearchParams();
  if (qOut) search.set("q", qOut);
  if (cities.length === 1) {
    search.set("city", cities[0]);
  } else if (cities.length > 1) {
    const packed = packCitiesForQuery(cities);
    if (packed) search.set("cities", packed);
  }
  if (params?.work_format) search.set("work_format", params.work_format);
  if (params?.price_min != null && params.price_min !== 0) {
    search.set("price_min", String(params.price_min));
  }
  if (params?.price_max != null && params.price_max !== 0) {
    search.set("price_max", String(params.price_max));
  }
  if (params?.page != null && params.page !== 0) {
    search.set("page", String(params.page));
  }
  if (params?.page_size != null && params.page_size !== 0) {
    search.set("page_size", String(params.page_size));
  }
  if (params?.limit != null) search.set("limit", String(params.limit));
  if (params?.offset != null) search.set("offset", String(params.offset));
  const qs = search.toString();
  return fetchAPI<{ masters: MasterProfile[]; total: number }>(
    `/api/v1/masters${qs ? `?${qs}` : ""}`,
    init,
  );
}

/** Как getVenues: список мастеров без фильтров поиска (только пагинация). */
export async function getPublicMastersCatalog(
  params?: { page?: number; page_size?: number },
  init?: RequestInit,
): Promise<{ masters: MasterProfile[]; total: number }> {
  const search = new URLSearchParams();
  if (params) {
    Object.entries(params).forEach(([k, v]) => {
      if (v === undefined || v === null) return
      if (typeof v === "string" && v === "") return
      search.set(k, String(v));
    });
  }
  const qs = search.toString();
  return fetchAPI<{ masters: MasterProfile[]; total: number }>(
    `/api/v1/masters${qs ? `?${qs}` : ""}`,
    init,
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

export async function getMyClientMasterBookings(params?: {
  status?: string;
}): Promise<{ bookings: MasterBooking[] }> {
  const search = new URLSearchParams();
  if (params?.status) search.set("status", params.status);
  const qs = search.toString();
  return fetchAPI<{ bookings: MasterBooking[] }>(
    `/api/v1/my/master-bookings${qs ? `?${qs}` : ""}`,
  );
}

/** Короткоживущий билет для WebSocket (если Redis недоступен на gateway — вернётся ошибка, клиент падает на access_token). */
export async function issueChatWsTicket(): Promise<{
  ticket: string;
  expires_in_sec: number;
} | null> {
  try {
    return await fetchAPI<{ ticket: string; expires_in_sec: number }>(chatV2Paths.wsTicket, {
      method: "POST",
      body: JSON.stringify({}),
    });
  } catch {
    return null;
  }
}

export async function ensureChatThreadV2(data: {
  kind: "venue_booking" | "master_booking";
  ref_id: string;
}): Promise<{ thread: ChatThread }> {
  return fetchAPI<{ thread: ChatThread }>(chatV2Paths.threadsEnsure, {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function listChatThreadsV2(params?: {
  limit?: number;
  offset?: number;
}): Promise<{ threads: ChatThread[]; total: number }> {
  const q = new URLSearchParams();
  if (params?.limit) q.set("limit", String(params.limit));
  if (params?.offset) q.set("offset", String(params.offset));
  const qs = q.toString();
  return fetchAPI<{ threads: ChatThread[]; total: number }>(
    `${chatV2Paths.threads}${qs ? `?${qs}` : ""}`,
  );
}

export async function listChatMessagesV2(
  threadId: string,
  params?: { limit?: number; offset?: number },
): Promise<{ messages: ChatMessage[]; total: number }> {
  const q = new URLSearchParams();
  if (params?.limit) q.set("limit", String(params.limit));
  if (params?.offset) q.set("offset", String(params.offset));
  const qs = q.toString();
  return fetchAPI<{ messages: ChatMessage[]; total: number }>(
    `${chatV2Paths.threadMessages(threadId)}${qs ? `?${qs}` : ""}`,
  );
}

export async function sendChatMessageV2(
  threadId: string,
  text: string,
  clientMsgId?: string,
): Promise<{ thread: ChatThread; message: ChatMessage }> {
  return fetchAPI<{ thread: ChatThread; message: ChatMessage }>(
    chatV2Paths.threadMessages(threadId),
    { method: "POST", body: JSON.stringify({ text, client_msg_id: clientMsgId ?? "" }) },
  );
}

export async function markChatThreadReadV2(
  threadId: string,
  read_upto_message_id?: string,
): Promise<{ thread: ChatThread }> {
  return fetchAPI<{ thread: ChatThread }>(
    chatV2Paths.threadRead(threadId),
    { method: "POST", body: JSON.stringify({ read_upto_message_id: read_upto_message_id ?? "" }) },
  );
}

export async function submitSupportContact(
  data: SupportContactRequest,
): Promise<SupportContactResponse> {
  return fetchAPI<SupportContactResponse>("/api/v1/support/contact", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function submitSupportTicketReply(
  data: SupportTicketReplyRequest,
): Promise<SupportTicketReplyResponse> {
  const body: Record<string, string> = {
    user_email: data.user_email.trim(),
    reply: data.reply.trim(),
  };
  const tn = data.ticket_number?.trim();
  const rid = data.request_id?.trim();
  if (tn) body.ticket_number = tn;
  if (rid) body.request_id = rid;
  return fetchAPI<SupportTicketReplyResponse>("/api/v1/admin/support/reply", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export async function listAdminSupportTickets(params?: {
  limit?: number;
  offset?: number;
}): Promise<SupportTicketsListResponse> {
  const q = new URLSearchParams();
  if (params?.limit != null) q.set("limit", String(params.limit));
  if (params?.offset != null) q.set("offset", String(params.offset));
  const qs = q.toString();
  return fetchAPI<SupportTicketsListResponse>(`/api/v1/admin/support/tickets${qs ? `?${qs}` : ""}`);
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

function looksSafeInvalidArgumentDetail(detail: string): boolean {
  const t = detail.trim();
  if (t.length === 0 || t.length > 800) return false;
  if (/^rpc error:/i.test(t) || /connection refused|ECONNRESET/i.test(t)) return false;
  return true;
}

/** Текст для UI из поля `error` шлюза при INVALID_ARGUMENT (gRPC message). */
function messageForInvalidArgumentDetail(detail: string): string | undefined {
  const t = detail.trim();
  if (!t) return undefined;
  if (/[а-яёА-ЯЁ]/.test(t)) {
    return t;
  }
  const lower = t.toLowerCase();
  if (lower.includes("password must be at least")) {
    return "Пароль: не менее 8 символов";
  }
  if (lower.includes("token and new_password")) {
    return "Укажите ссылку из письма и новый пароль";
  }
  if (lower.includes("profile cannot be submitted in current status")) {
    return "Отправить на модерацию сейчас нельзя (статус профиля не подходит). Обновите страницу: возможно, профиль уже на проверке или опубликован.";
  }
  if (lower.includes("invalid travel_exclude_zones")) {
    return "Некорректный формат зон «куда не выезжаю». Сохраните профиль ещё раз с этой страницы.";
  }
  if (lower.includes("invalid service id")) {
    return "Некорректный идентификатор услуги. Обновите страницу и заново сохраните список услуг.";
  }
  if (lower === "invalid payout_legal_form" || lower.includes("invalid payout_legal_form")) {
    return "Некорректная форма выплат. Выберите в профиле: ИП, ООО, физическое лицо или самозанятость и сохраните снова.";
  }
  if (looksSafeInvalidArgumentDetail(t)) {
    return t;
  }
  return undefined;
}

export function formatApiErrorMessage(e: unknown, fallback: string): string {
  if (e instanceof ApiError) {
    if (e.code === "GATEWAY.UPSTREAM.INVALID_ARGUMENT" && e.gatewayDetail) {
      const mapped = messageForInvalidArgumentDetail(e.gatewayDetail);
      if (mapped) return mapped;
    }
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
