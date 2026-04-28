import { describe, it, expect, beforeEach, vi, type Mock } from "vitest";
import { login, register, getVenues, ApiError, formatApiErrorMessage } from "@/lib/api";

const API_URL = "http://localhost:8080";

let fetchMock: Mock;

beforeEach(() => {
  localStorage.clear();
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
    fetchMock.mockResolvedValueOnce(
      mockResponse(401, {
        code: "GATEWAY.UPSTREAM.UNAUTHENTICATED",
        error: "rpc error: code = Unauthenticated desc = INTERNAL LEAK",
      }),
    );

    try {
      await login({ email: "a@b.com", password: "bad" });
      expect.fail("expected throw");
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError);
      const err = e as ApiError;
      expect(err.code).toBe("GATEWAY.UPSTREAM.UNAUTHENTICATED");
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
    localStorage.setItem("token", "my-jwt");
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
    localStorage.setItem("token", "expired-tok");
    localStorage.setItem("refresh_token", "valid-refresh");

    fetchMock
      .mockResolvedValueOnce(mockResponse(401, "expired"))
      .mockResolvedValueOnce(
        mockResponse(200, {
          access_token: "new-tok",
          refresh_token: "new-ref",
        }),
      )
      .mockResolvedValueOnce(
        mockResponse(200, { venues: [], total: 0, page: 1, page_size: 10 }),
      );

    const result = await getVenues({ page: 1 });

    expect(result.venues).toEqual([]);
    expect(localStorage.getItem("token")).toBe("new-tok");
    expect(localStorage.getItem("refresh_token")).toBe("new-ref");
  });

  it("throws when refresh fails and original request was 401", async () => {
    localStorage.setItem("token", "expired-tok");
    localStorage.setItem("refresh_token", "bad-refresh");

    const originalHref = window.location.href;
    Object.defineProperty(window, "location", {
      writable: true,
      value: { href: originalHref },
    });

    fetchMock
      .mockResolvedValueOnce(mockResponse(401, "expired"))
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
