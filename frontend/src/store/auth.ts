import { create } from "zustand";
import type { User } from "@/lib/types";

/** Есть ли полноценная сессия для UI (шапка, колокольчик и т.д.). */
export function hasAuthSession(user: User | null, token: string | null): boolean {
  const t = token?.trim();
  if (!t) return false;
  const id = user?.id?.trim();
  return Boolean(id);
}

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
    let user: User | null = null;
    if (userStr) {
      try {
        user = JSON.parse(userStr) as User;
      } catch {
        user = null;
      }
    }
    if (user && !user.id?.trim()) {
      user = null;
      localStorage.removeItem("user");
    }
    if (!token?.trim() && user) {
      localStorage.removeItem("user");
      user = null;
    }
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
