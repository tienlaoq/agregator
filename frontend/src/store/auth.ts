import { create } from "zustand";
import type { User } from "@/lib/types";

/** Есть ли полноценная сессия для UI (шапка, колокольчик и т.д.). */
export function hasAuthSession(user: User | null, token: string | null): boolean {
  const t = token?.trim();
  if (!t) return false;
  const id = user?.id?.trim();
  return Boolean(id);
}

/**
 * Читает legacy-данные сессии из localStorage и сразу вычищает их оттуда:
 * access token больше не хранится в localStorage (живёт только в памяти),
 * refresh token переехал в httpOnly cookie. Возвращает то, что удалось
 * мигрировать. Чистая функция без побочных эффектов на состояние стора.
 */
function loadLegacySession(): { token: string | null; user: User | null } {
  if (typeof window === "undefined") return { token: null, user: null };

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
  // Access token тоже убираем из localStorage — в новых сессиях он только в памяти.
  if (token) localStorage.removeItem("token");

  return { token, user };
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
  /**
   * Восстанавливает сессию при загрузке приложения: мигрирует legacy-данные,
   * затем делает «тихий» refresh через httpOnly cookie и подтягивает профиль.
   * `hydrated` переключается в true только после завершения, чтобы защищённые
   * страницы не редиректили на логин до попытки восстановления.
   */
  bootstrap: () => Promise<void>;
  login: (token: string, refreshToken: string, user: User) => Promise<void>;
  setTokens: (token: string, refreshToken: string) => Promise<void>;
  logout: () => Promise<void>;
  setUser: (user: User) => void;
}

export const useAuthStore = create<AuthState>((set, get) => ({
  user: null,
  token: null,
  refreshToken: null,
  hydrated: false,

  hydrate: () => {
    if (typeof window === "undefined") return;
    const { token, user } = loadLegacySession();
    set({ token, user, hydrated: true });
  },

  bootstrap: async () => {
    if (typeof window === "undefined") return;

    // 1. Не затираем сессию, уже поднятую в памяти (например, OAuth-callback
    //    синхронно проставляет access-токен ДО того, как отработает bootstrap —
    //    дочерние эффекты React выполняются раньше родительских). Legacy-данные
    //    из localStorage мигрируем только если токена в памяти ещё нет.
    if (!get().token?.trim()) {
      const legacy = loadLegacySession();
      if (legacy.token || legacy.user) {
        set({ token: legacy.token, user: legacy.user });
      }
    }

    // 2. Нет access-токена в памяти — пробуем «тихо» поднять сессию из
    //    httpOnly refresh-cookie. Делаем это напрямую (не через getProfile),
    //    чтобы для анонимных пользователей не сработал жёсткий редирект на логин.
    //
    //    ВАЖНО: на /auth/callback тихий refresh НЕ запускаем. Там сессию
    //    устанавливает сам обработчик callback, а refresh-токен одноразовый
    //    (ротируется при каждом использовании) — параллельный вызов «съел» бы
    //    токен из-под обработчика и привёл бы к 401.
    const onOAuthCallback =
      window.location.pathname.startsWith("/auth/callback");
    if (!get().token?.trim() && !onOAuthCallback) {
      try {
        const res = await fetch("/api/auth/refresh", { method: "POST" });
        if (res.ok) {
          const data = (await res.json()) as { access_token?: string };
          if (data.access_token) {
            set({ token: data.access_token });
          }
        }
      } catch {
        // Сеть недоступна — считаем пользователя анонимным, не блокируем загрузку.
      }
    }

    // 3. Токен есть, но профиль не восстановлен — подтягиваем его.
    if (get().token?.trim() && !get().user) {
      try {
        const { getProfile } = await import("@/lib/api");
        const me = await getProfile();
        localStorage.setItem("user", JSON.stringify(me));
        set({ user: me });
      } catch {
        // Токен невалиден / профиль недоступен — сбрасываем в анонимное состояние.
        set({ token: null, user: null });
      }
    }

    set({ hydrated: true });
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
