import { create } from "zustand";
import type { User } from "@/lib/types";

interface AuthState {
  user: User | null;
  token: string | null;
  refreshToken: string | null;
  hydrated: boolean;
  hydrate: () => void;
  login: (token: string, refreshToken: string, user: User) => void;
  setTokens: (token: string, refreshToken: string) => void;
  logout: () => void;
  setUser: (user: User) => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  token: null,
  refreshToken: null,
  hydrated: false,
  hydrate: () => {
    if (typeof window === "undefined") return;
    const token = localStorage.getItem("token");
    const refreshToken = localStorage.getItem("refresh_token");
    const userStr = localStorage.getItem("user");
    const user = userStr ? (JSON.parse(userStr) as User) : null;
    set({ token, refreshToken, user, hydrated: true });
  },
  login: (token, refreshToken, user) => {
    localStorage.setItem("token", token);
    localStorage.setItem("refresh_token", refreshToken);
    localStorage.setItem("user", JSON.stringify(user));
    set({ token, refreshToken, user });
  },
  setTokens: (token, refreshToken) => {
    localStorage.setItem("token", token);
    localStorage.setItem("refresh_token", refreshToken);
    set({ token, refreshToken });
  },
  logout: () => {
    localStorage.removeItem("token");
    localStorage.removeItem("refresh_token");
    localStorage.removeItem("user");
    set({ token: null, refreshToken: null, user: null });
  },
  setUser: (user) => {
    localStorage.setItem("user", JSON.stringify(user));
    set({ user });
  },
}));
