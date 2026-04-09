import type {
  AuthResponse,
  Booking,
  CreateBookingRequest,
  CreateReviewRequest,
  CreateVenueRequest,
  LoginRequest,
  RegisterRequest,
  Review,
  User,
  Venue,
} from "./types";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

let refreshPromise: Promise<boolean> | null = null;

async function tryRefreshToken(): Promise<boolean> {
  const refreshToken =
    typeof window !== "undefined" ? localStorage.getItem("refresh_token") : null;
  if (!refreshToken) return false;

  try {
    const res = await fetch(`${API_URL}/api/v1/auth/refresh`, {
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

  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
  };

  const res = await fetch(`${API_URL}${path}`, {
    ...options,
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
      const retryRes = await fetch(`${API_URL}${path}`, {
        ...options,
        headers: {
          "Content-Type": "application/json",
          ...(options?.headers as Record<string, string>),
          ...(newToken ? { Authorization: `Bearer ${newToken}` } : {}),
        },
      });

      if (!retryRes.ok) {
        const text = await retryRes.text().catch(() => "Unknown error");
        throw new ApiError(retryRes.status, text);
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
    const text = await res.text().catch(() => "Unknown error");
    throw new ApiError(res.status, text);
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

interface PaginatedVenues {
  venues: Venue[];
  page: number;
  page_size: number;
  total: number;
}

export async function getVenues(params?: {
  page?: number;
  page_size?: number;
  type?: string;
  sort_by?: string;
}): Promise<PaginatedVenues> {
  const search = new URLSearchParams();
  if (params) {
    Object.entries(params).forEach(([k, v]) => {
      if (v !== undefined && v !== "") search.set(k, String(v));
    });
  }
  const qs = search.toString();
  return fetchAPI<PaginatedVenues>(`/api/v1/venues${qs ? `?${qs}` : ""}`);
}

export async function searchVenues(params: {
  q?: string;
  type?: string;
  price_min?: number;
  price_max?: number;
  rating_min?: number;
  page?: number;
  page_size?: number;
}): Promise<PaginatedVenues> {
  const search = new URLSearchParams();
  Object.entries(params).forEach(([k, v]) => {
    if (v !== undefined && v !== "" && v !== 0) search.set(k, String(v));
  });
  const qs = search.toString();
  return fetchAPI<PaginatedVenues>(`/api/v1/venues/search${qs ? `?${qs}` : ""}`);
}

export async function getVenueBySlug(slug: string): Promise<Venue> {
  return fetchAPI<Venue>(`/api/v1/venues/${slug}`);
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

export async function createVenue(data: CreateVenueRequest): Promise<Venue> {
  return fetchAPI<Venue>("/api/v1/venues", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

// Admin API
export async function getAdminVenues(params?: {
  status?: string;
  page?: number;
  page_size?: number;
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
  action: "approve" | "reject" | "suspend",
  comment?: string,
): Promise<Venue> {
  return fetchAPI<Venue>(`/api/v1/admin/venues/${venueId}/moderate`, {
    method: "POST",
    body: JSON.stringify({ action, comment: comment || "" }),
  });
}

export { ApiError };
