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
  it("login stores token, refreshToken and user", () => {
    useAuthStore.getState().login("access-tok", "refresh-tok", testUser);

    const state = useAuthStore.getState();
    expect(state.token).toBe("access-tok");
    expect(state.refreshToken).toBe("refresh-tok");
    expect(state.user).toEqual(testUser);
    expect(localStorage.getItem("token")).toBe("access-tok");
    expect(localStorage.getItem("refresh_token")).toBe("refresh-tok");
    expect(JSON.parse(localStorage.getItem("user")!)).toEqual(testUser);
  });

  it("logout clears all data", () => {
    useAuthStore.getState().login("tok", "ref", testUser);
    useAuthStore.getState().logout();

    const state = useAuthStore.getState();
    expect(state.token).toBeNull();
    expect(state.refreshToken).toBeNull();
    expect(state.user).toBeNull();
    expect(localStorage.getItem("token")).toBeNull();
    expect(localStorage.getItem("refresh_token")).toBeNull();
    expect(localStorage.getItem("user")).toBeNull();
  });

  it("hydrate reads from localStorage", () => {
    localStorage.setItem("token", "hydrated-tok");
    localStorage.setItem("refresh_token", "hydrated-ref");
    localStorage.setItem("user", JSON.stringify(testUser));

    useAuthStore.getState().hydrate();

    const state = useAuthStore.getState();
    expect(state.token).toBe("hydrated-tok");
    expect(state.refreshToken).toBe("hydrated-ref");
    expect(state.user).toEqual(testUser);
    expect(state.hydrated).toBe(true);
  });

  it("hydrate with empty localStorage", () => {
    useAuthStore.getState().hydrate();

    const state = useAuthStore.getState();
    expect(state.token).toBeNull();
    expect(state.refreshToken).toBeNull();
    expect(state.user).toBeNull();
    expect(state.hydrated).toBe(true);
  });

  it("setTokens updates tokens without touching user", () => {
    useAuthStore.getState().login("old-tok", "old-ref", testUser);
    useAuthStore.getState().setTokens("new-tok", "new-ref");

    const state = useAuthStore.getState();
    expect(state.token).toBe("new-tok");
    expect(state.refreshToken).toBe("new-ref");
    expect(state.user).toEqual(testUser);
    expect(localStorage.getItem("token")).toBe("new-tok");
    expect(localStorage.getItem("refresh_token")).toBe("new-ref");
  });

  it("setUser updates user in state and localStorage", () => {
    const updatedUser = { ...testUser, name: "Обновлённый" };
    useAuthStore.getState().setUser(updatedUser);

    expect(useAuthStore.getState().user).toEqual(updatedUser);
    expect(JSON.parse(localStorage.getItem("user")!).name).toBe("Обновлённый");
  });
});
