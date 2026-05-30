import { describe, it, expect, beforeEach, vi, type Mock } from "vitest";
import { login, register, getVenues, ApiError, formatApiErrorMessage } from "@/lib/api";
import { useAuthStore } from "@/store/auth";

const API_URL = "http://localhost:8080";

let fetchMock: Mock;

beforeEach(() => {
  localStorage.clear();
  // Access token lives only in memory now — reset the store between tests.
  useAuthStore.setState({ token: null, user: null });
  fetchMock = vi.fn();
  global.fetch = fetchMock;
});

function mockResponse(status: number, body: unknown) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(JSON.stringify(body)),
  };
}

describe("login", () => {
  it("returns AuthResponse on success", async () => {
    const authResp = {
      access_token: "at",
      refresh_token: "rt",
      user_id: "u1",
    };
    fetchMock.mockResolvedValueOnce(mockResponse(200, authResp));

    const result = await login({ email: "a@b.com", password: "pass" });

    expect(result).toEqual(authResp);
    expect(fetchMock).toHaveBeenCalledWith(
      `${API_URL}/api/v1/auth/login`,
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("throws ApiError on failure", async () => {
    fetchMock.mockResolvedValueOnce(mockResponse(401, "unauthorized"));

    await expect(login({ email: "a@b.com", password: "bad" })).rejects.toThrow(
      ApiError,
    );
  });

  it("parses gateway code from JSON error body", async () => {
    // Use 403 (not 401): a 401 in the browser now triggers the silent
    // auto-refresh flow and surfaces "session_expired" instead of the
    // upstream gateway code. Any other status passes the gateway error through.
    fetchMock.mockResolvedValueOnce(
      mockResponse(403, {
        code: "GATEWAY.UPSTREAM.PERMISSION_DENIED",
        error: "rpc error: code = PermissionDenied desc = INTERNAL LEAK",
      }),
    );

    try {
      await login({ email: "a@b.com", password: "bad" });
      expect.fail("expected throw");
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError);
      const err = e as ApiError;
      expect(err.code).toBe("GATEWAY.UPSTREAM.PERMISSION_DENIED");
      expect(formatApiErrorMessage(err, "fallback")).not.toContain("rpc");
      expect(formatApiErrorMessage(err, "fallback")).not.toContain("INTERNAL");
    }
  });
});

describe("register", () => {
  it("returns AuthResponse on success", async () => {
    const authResp = {
      access_token: "at",
      refresh_token: "rt",
      user_id: "u2",
    };
    fetchMock.mockResolvedValueOnce(mockResponse(201, authResp));

    const result = await register({
      name: "Test",
      email: "t@t.com",
      password: "pass",
      role: "user",
    });
    expect(result).toEqual(authResp);
  });
});

describe("fetchAPI authorization", () => {
  it("adds Authorization header when token exists", async () => {
    // Access token now lives in the Zustand store (memory), not localStorage.
    useAuthStore.setState({ token: "my-jwt" });
    fetchMock.mockResolvedValueOnce(
      mockResponse(200, { venues: [], total: 0, page: 1, page_size: 10 }),
    );

    await getVenues({ page: 1, page_size: 10 });

    const [, options] = fetchMock.mock.calls[0];
    expect(options.headers.Authorization).toBe("Bearer my-jwt");
  });

  it("omits Authorization header when no token", async () => {
    fetchMock.mockResolvedValueOnce(
      mockResponse(200, { venues: [], total: 0, page: 1, page_size: 10 }),
    );

    await getVenues();

    const [, options] = fetchMock.mock.calls[0];
    expect(options.headers.Authorization).toBeUndefined();
  });
});

describe("fetchAPI auto-refresh on 401", () => {
  it("retries request after successful token refresh", async () => {
    // Refresh flow: refresh_token is in an httpOnly cookie. The browser calls
    // /api/auth/refresh, then setTokens() persists the new access token via
    // /api/auth/set-refresh, then the original request is retried.
    useAuthStore.setState({ token: "expired-tok" });

    fetchMock
      // 1) original request → 401
      .mockResolvedValueOnce(mockResponse(401, "expired"))
      // 2) POST /api/auth/refresh → new access token
      .mockResolvedValueOnce(mockResponse(200, { access_token: "new-tok" }))
      // 3) POST /api/auth/set-refresh (inside setTokens) → ok
      .mockResolvedValueOnce(mockResponse(200, { ok: true }))
      // 4) retried original request → success
      .mockResolvedValueOnce(
        mockResponse(200, { venues: [], total: 0, page: 1, page_size: 10 }),
      );

    const result = await getVenues({ page: 1 });

    expect(result.venues).toEqual([]);
    // New access token lives in the store, never in localStorage.
    expect(useAuthStore.getState().token).toBe("new-tok");
    expect(localStorage.getItem("token")).toBeNull();
  });

  it("throws when refresh fails and original request was 401", async () => {
    useAuthStore.setState({ token: "expired-tok" });

    Object.defineProperty(window, "location", {
      writable: true,
      value: { href: "" },
    });

    fetchMock
      // 1) original request → 401
      .mockResolvedValueOnce(mockResponse(401, "expired"))
      // 2) POST /api/auth/refresh → 401 (refresh rejected)
      .mockResolvedValueOnce(mockResponse(401, "refresh failed"));

    await expect(getVenues({ page: 1 })).rejects.toThrow();
  });
});

describe("formatApiErrorMessage", () => {
  it("maps unknown gateway code by HTTP status", () => {
    const err = new ApiError(
      418,
      JSON.stringify({ code: "GATEWAY.FUTURE.UNKNOWN", error: "tea pot" }),
      "GATEWAY.FUTURE.UNKNOWN",
    );
    expect(formatApiErrorMessage(err, "Свой текст")).toBe("Свой текст");
  });

  it("shows Russian gateway detail for INVALID_ARGUMENT (e.g. password reset)", () => {
    const err = new ApiError(
      400,
      JSON.stringify({
        code: "GATEWAY.UPSTREAM.INVALID_ARGUMENT",
        error: "Ссылка сброса недействительна или истекла",
      }),
      "GATEWAY.UPSTREAM.INVALID_ARGUMENT",
      "Ссылка сброса недействительна или истекла",
    );
    expect(formatApiErrorMessage(err, "fallback")).toBe("Ссылка сброса недействительна или истекла");
  });

  it("maps English password hint from gateway detail", () => {
    const err = new ApiError(
      400,
      JSON.stringify({
        code: "GATEWAY.UPSTREAM.INVALID_ARGUMENT",
        error: "password must be at least 8 characters",
      }),
      "GATEWAY.UPSTREAM.INVALID_ARGUMENT",
      "password must be at least 8 characters",
    );
    expect(formatApiErrorMessage(err, "fallback")).toBe("Пароль: не менее 8 символов");
  });

  it("maps master submit English status message to Russian", () => {
    const err = new ApiError(
      400,
      JSON.stringify({
        code: "GATEWAY.UPSTREAM.INVALID_ARGUMENT",
        error: "profile cannot be submitted in current status: active",
      }),
      "GATEWAY.UPSTREAM.INVALID_ARGUMENT",
      "profile cannot be submitted in current status: active",
    );
    const msg = formatApiErrorMessage(err, "fallback");
    expect(msg).toMatch(/статус профиля/i);
    expect(msg).not.toBe("Проверьте введённые данные и попробуйте снова.");
  });

  it("shows other short English INVALID_ARGUMENT details from upstream", () => {
    const err = new ApiError(
      400,
      JSON.stringify({
        code: "GATEWAY.UPSTREAM.INVALID_ARGUMENT",
        error: "display_name is required",
      }),
      "GATEWAY.UPSTREAM.INVALID_ARGUMENT",
      "display_name is required",
    );
    expect(formatApiErrorMessage(err, "fallback")).toBe("display_name is required");
  });

  it("detects network-style client errors", () => {
    expect(
      formatApiErrorMessage(new TypeError("Failed to fetch"), "другое"),
    ).toMatch(/соединени/i);
  });
});
