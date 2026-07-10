"use client";

import type { ReactNode } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { useAuthStore } from "@/store/auth";
import {
  LayoutDashboard,
  CalendarDays,
  Bell,
  Settings,
  LogOut,
  type LucideIcon,
} from "lucide-react";

type NavItem = {
  label: string;
  icon: LucideIcon;
  href: string;
  /** точное совпадение пути (для индекса /my, который является префиксом остальных) */
  exact?: boolean;
};

const NAV: NavItem[] = [
  { label: "Обзор", icon: LayoutDashboard, href: "/my", exact: true },
  { label: "Бронирования", icon: CalendarDays, href: "/my/bookings" },
  { label: "Уведомления", icon: Bell, href: "/my/notifications" },
  { label: "Настройки", icon: Settings, href: "/my/profile" },
];

function roleLabel(role: string | undefined): string {
  switch (role) {
    case "venue_owner":
      return "Владелец";
    case "master":
      return "Пар-мастер";
    case "admin":
      return "Администратор";
    default:
      return "Посетитель";
  }
}

/**
 * Оболочка личного кабинета обычного пользователя: боковое меню (десктоп),
 * горизонтальная навигация (мобильные) и область контента. Применяется через
 * layout ко всем страницам `/my/*`.
 */
export function UserWorkspaceShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const user = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);

  const name = user?.name || "Гость";

  function isActive(item: NavItem): boolean {
    return item.exact ? pathname === item.href : pathname.startsWith(item.href);
  }

  function handleLogout() {
    logout();
    router.push("/");
  }

  return (
    <div className="flex min-h-[calc(100vh-4rem)]">
      <aside className="hidden w-56 shrink-0 flex-col gap-1 border-r border-border bg-muted/20 p-3 md:flex">
        <div className="mb-3 flex items-center gap-3 px-2 py-2">
          <Avatar className="h-10 w-10">
            <AvatarImage src={user?.avatar_url} alt={name} />
            <AvatarFallback className="bg-primary text-primary-foreground">
              {name.charAt(0).toUpperCase()}
            </AvatarFallback>
          </Avatar>
          <div className="min-w-0">
            <div className="truncate text-sm font-medium">{name}</div>
            <div className="text-xs text-muted-foreground">{roleLabel(user?.role)}</div>
          </div>
        </div>

        {NAV.map((item) => {
          const active = isActive(item);
          return (
            <Link
              key={item.href}
              href={item.href}
              className={`flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm ${
                active
                  ? "bg-primary/10 font-medium text-primary"
                  : "text-muted-foreground hover:bg-muted"
              }`}
            >
              <item.icon className="h-4 w-4 shrink-0" />
              <span className="flex-1 truncate">{item.label}</span>
            </Link>
          );
        })}

        <div className="my-1 border-t border-border" />
        <button
          type="button"
          onClick={handleLogout}
          className="flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm text-destructive hover:bg-destructive/10"
        >
          <LogOut className="h-4 w-4 shrink-0" />
          <span className="flex-1 truncate text-left">Выйти</span>
        </button>
      </aside>

      <div className="min-w-0 flex-1">
        <nav className="flex gap-1 overflow-x-auto border-b border-border px-2 py-2 md:hidden">
          {NAV.map((item) => {
            const active = isActive(item);
            return (
              <Link
                key={item.href}
                href={item.href}
                className={`flex shrink-0 items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm ${
                  active
                    ? "bg-primary/10 font-medium text-primary"
                    : "text-muted-foreground hover:bg-muted"
                }`}
              >
                <item.icon className="h-4 w-4 shrink-0" />
                {item.label}
              </Link>
            );
          })}
        </nav>
        {children}
      </div>
    </div>
  );
}
