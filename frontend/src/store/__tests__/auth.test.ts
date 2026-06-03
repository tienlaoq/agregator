import { describe, it, expect, beforeEach, vi, type Mock } from "vitest";
import { useAuthStore } from "@/store/auth";
import type { User } from "@/lib/types";

const testUser: User = {
  id: "user-1",
  email: "test@example.com",
  name: "Тест",
  phone: "+7 (999) 123-45-67",
  role: "user",
};

let fetchMock: Mock;

function mockResponse(status: number, body: unknown) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(JSON.stringify(body)),
  };
}

beforeEach(() => {
  localStorage.clear();
  useAuthStore.setState({
    user: null,
    token: null,
    refreshToken: null,
    hydrated: false,
  });
  fetchMock = vi.fn();
  global.fetch = fetchMock;
});

describe("useAuthStore", () => {
  it("login stores access token (memory) and user; never persists tokens", async () => {
    await useAuthStore.getState().login("access-tok", "refresh-tok", testUser);

    const state = useAuthStore.getState();
    expect(state.token).toBe("access-tok");
    // refreshToken is deprecated — refresh token lives in an httpOnly cookie.
    expect(state.refreshToken).toBeNull();
    expect(state.user).toEqual(testUser);
    // Access token lives only in memory; refresh token only in httpOnly cookie.
    expect(localStorage.getItem("token")).toBeNull();
    expect(localStorage.getItem("refresh_token")).toBeNull();
    // Only the non-secret user profile is persisted.
    expect(JSON.parse(localStorage.getItem("user")!)).toEqual(testUser);
  });

  it("logout clears all data", async () => {
    await useAuthStore.getState().login("tok", "ref", testUser);
    await useAuthStore.getState().logout();

    const state = useAuthStore.getState();
    expect(state.token).toBeNull();
    expect(state.refreshToken).toBeNull();
    expect(state.user).toBeNull();
    expect(localStorage.getItem("token")).toBeNull();
    expect(localStorage.getItem("refresh_token")).toBeNull();
    expect(localStorage.getItem("user")).toBeNull();
  });

  it("hydrate migrates legacy token out of localStorage into memory", () => {
    // Legacy sessions stored the access token in localStorage; hydrate reads it
    // into memory and evicts it (and any stale refresh_token) from storage.
    localStorage.setItem("token", "hydrated-tok");
    localStorage.setItem("refresh_token", "stale-ref");
    localStorage.setItem("user", JSON.stringify(testUser));

    useAuthStore.getState().hydrate();

    const state = useAuthStore.getState();
    expect(state.token).toBe("hydrated-tok");
    expect(state.refreshToken).toBeNull();
    expect(state.user).toEqual(testUser);
    expect(state.hydrated).toBe(true);
    expect(localStorage.getItem("token")).toBeNull();
    expect(localStorage.getItem("refresh_token")).toBeNull();
  });

  it("hydrate with empty localStorage", () => {
    useAuthStore.getState().hydrate();

    const state = useAuthStore.getState();
    expect(state.token).toBeNull();
    expect(state.refreshToken).toBeNull();
    expect(state.user).toBeNull();
    expect(state.hydrated).toBe(true);
  });

  it("setTokens updates access token without touching user", async () => {
    await useAuthStore.getState().login("old-tok", "old-ref", testUser);
    await useAuthStore.getState().setTokens("new-tok", "new-ref");

    const state = useAuthStore.getState();
    expect(state.token).toBe("new-tok");
    expect(state.refreshToken).toBeNull();
    expect(state.user).toEqual(testUser);
    // Access token stays in memory; refresh token stays in the httpOnly cookie.
    expect(localStorage.getItem("token")).toBeNull();
    expect(localStorage.getItem("refresh_token")).toBeNull();
  });

  it("bootstrap restores session via silent refresh when no token in memory", async () => {
    // No access token in memory (simulates a page refresh). The httpOnly
    // refresh cookie is valid → /api/auth/refresh returns a fresh access token,
    // then GET /users/me restores the profile.
    fetchMock.mockImplementation((url: string) => {
      if (url === "/api/auth/refresh") {
        return Promise.resolve(mockResponse(200, { access_token: "fresh-tok" }));
      }
      if (url.endsWith("/api/v1/users/me")) {
        return Promise.resolve(mockResponse(200, testUser));
      }
      return Promise.resolve(mockResponse(404, {}));
    });

    await useAuthStore.getState().bootstrap();

    const state = useAuthStore.getState();
    expect(state.token).toBe("fresh-tok");
    expect(state.user).toEqual(testUser);
    expect(state.hydrated).toBe(true);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/auth/refresh",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("bootstrap stays anonymous (no redirect) when refresh cookie is absent", async () => {
    // Refresh cookie missing/expired → /api/auth/refresh returns 401. The user
    // is simply anonymous; bootstrap must not call /users/me nor blow up.
    fetchMock.mockImplementation((url: string) => {
      if (url === "/api/auth/refresh") {
        return Promise.resolve(mockResponse(401, { error: "no_refresh_token" }));
      }
      return Promise.resolve(mockResponse(404, {}));
    });

    await useAuthStore.getState().bootstrap();

    const state = useAuthStore.getState();
    expect(state.token).toBeNull();
    expect(state.user).toBeNull();
    expect(state.hydrated).toBe(true);
    // Only the refresh probe should have been attempted — no profile fetch.
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("bootstrap does not clobber a token already set in memory (OAuth callback)", async () => {
    // OAuth callback sets the access token synchronously before bootstrap runs.
    // bootstrap must NOT reset it to null from empty localStorage, and must NOT
    // hit /api/auth/refresh (which would rotate the single-use refresh cookie).
    useAuthStore.setState({ token: "oauth-tok", user: testUser });

    await useAuthStore.getState().bootstrap();

    const state = useAuthStore.getState();
    expect(state.token).toBe("oauth-tok");
    expect(state.user).toEqual(testUser);
    expect(state.hydrated).toBe(true);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("bootstrap skips silent refresh on the /auth/callback route", async () => {
    // The callback handler owns session establishment there; a parallel refresh
    // would consume the rotating refresh token out from under it.
    const original = window.location.pathname;
    Object.defineProperty(window, "location", {
      value: { ...window.location, pathname: "/auth/callback" },
      writable: true,
    });

    await useAuthStore.getState().bootstrap();

    expect(fetchMock).not.toHaveBeenCalled();
    expect(useAuthStore.getState().hydrated).toBe(true);

    Object.defineProperty(window, "location", {
      value: { ...window.location, pathname: original },
      writable: true,
    });
  });

  it("setUser updates user in state and localStorage", () => {
    const updatedUser = { ...testUser, name: "Обновлённый" };
    useAuthStore.getState().setUser(updatedUser);

    expect(useAuthStore.getState().user).toEqual(updatedUser);
    expect(JSON.parse(localStorage.getItem("user")!).name).toBe("Обновлённый");
  });
});
