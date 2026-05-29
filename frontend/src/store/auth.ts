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
  /**
   * Access token — только в памяти (никогда в localStorage).
   * Refresh token — в httpOnly cookie, управляется через /api/auth/set-refresh.
   */
  token: string | null;
  /** @deprecated Не используется после миграции на httpOnly cookie. Оставлено для совместимости. */
  refreshToken: null;
  hydrated: boolean;
  hydrate: () => void;
  login: (token: string, refreshToken: string, user: User) => Promise<void>;
  setTokens: (token: string, refreshToken: string) => Promise<void>;
  logout: () => Promise<void>;
  setUser: (user: User) => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  token: null,
  refreshToken: null,
  hydrated: false,

  hydrate: () => {
    if (typeof window === "undefined") return;
    // Access token читаем из localStorage для обратной совместимости с уже залогиненными сессиями.
    // В новых сессиях login() не записывает его туда — token живёт только в памяти.
    const token = localStorage.getItem("token") ?? null;
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
    // Чистим устаревший refresh_token из localStorage (был уязвимостью).
    localStorage.removeItem("refresh_token");
    // Access token тоже убираем из localStorage после hydration — в будущих сессиях он только в памяти.
    if (token) localStorage.removeItem("token");
    set({ token, user, hydrated: true });
  },

  login: async (token, refreshToken, user) => {
    // Access token — только в памяти Zustand.
    // Refresh token — в httpOnly cookie через Next.js API route.
    // При OAuth-логине refresh_token уже установлен gateway-ом как HttpOnly
    // cookie, поэтому refreshToken может быть пустой строкой — в этом случае
    // вызов set-refresh пропускается.
    if (refreshToken) {
      try {
        await fetch("/api/auth/set-refresh", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ refresh_token: refreshToken }),
        });
      } catch {
        // Не блокируем логин если route недоступен — деградируем грейсфулли.
      }
    }
    // Пользователя сохраняем в localStorage (не секрет — только публичные поля профиля).
    localStorage.setItem("user", JSON.stringify(user));
    set({ token, user });
  },

  setTokens: async (token, refreshToken) => {
    try {
      await fetch("/api/auth/set-refresh", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ refresh_token: refreshToken }),
      });
    } catch {
      // Игнорируем сетевую ошибку при обновлении cookie.
    }
    set({ token });
  },

  logout: async () => {
    try {
      await fetch("/api/auth/set-refresh", { method: "DELETE" });
    } catch {
      // Игнорируем — cookie всё равно протухнет.
    }
    localStorage.removeItem("user");
    set({ token: null, user: null });
  },

  setUser: (user) => {
    localStorage.setItem("user", JSON.stringify(user));
    set({ user });
  },
}));
