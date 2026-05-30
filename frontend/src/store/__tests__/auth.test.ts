import { describe, it, expect, beforeEach } from "vitest";
import { useAuthStore } from "@/store/auth";
import type { User } from "@/lib/types";

const testUser: User = {
  id: "user-1",
  email: "test@example.com",
  name: "Тест",
  phone: "+7 (999) 123-45-67",
  role: "user",
};

beforeEach(() => {
  localStorage.clear();
  useAuthStore.setState({
    user: null,
    token: null,
    refreshToken: null,
    hydrated: false,
  });
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

  it("setUser updates user in state and localStorage", () => {
    const updatedUser = { ...testUser, name: "Обновлённый" };
    useAuthStore.getState().setUser(updatedUser);

    expect(useAuthStore.getState().user).toEqual(updatedUser);
    expect(JSON.parse(localStorage.getItem("user")!).name).toBe("Обновлённый");
  });
});
