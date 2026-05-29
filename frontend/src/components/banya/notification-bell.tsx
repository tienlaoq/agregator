"use client";

import { Bell } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { useAuthStore, hasAuthSession } from "@/store/auth";

type NotificationBellProps = {
  /** Позже — из API / стора; пока 0. */
  unreadCount?: number;
};

function notificationSubtitle(userRole: string | undefined): string {
  switch (userRole) {
    case "venue_owner":
      return "Брони по вашим заведениям, модерация карточек, задачи команды и слоты.";
    case "master":
      return "Брони с вашим участием, заявки гостей и статус модерации профиля.";
    case "admin":
      return "Очередь модерации заведений и мастеров, обращения и служебные сообщения.";
    case "user":
    default:
      return "";
  }
}

/**
 * Виден только при живой сессии (токен + пользователь с id в сторе).
 * Не зависит от пропа `isLoggedIn` шапки — один источник правды.
 */
export function NotificationBell({ unreadCount = 0 }: NotificationBellProps) {
  const user = useAuthStore((s) => s.user);
  const token = useAuthStore((s) => s.token);
  const hydrated = useAuthStore((s) => s.hydrated);

  if (!hydrated || !hasAuthSession(user, token)) return null;

  const hasUnread = unreadCount > 0;
  const subtitle = notificationSubtitle(user?.role);

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="relative h-9 w-9 shrink-0 text-muted-foreground hover:text-foreground"
          aria-label="Уведомления"
        >
          <Bell className="h-5 w-5" />
          {hasUnread ? (
            <span className="absolute right-1.5 top-1.5 flex h-2 w-2 rounded-full bg-destructive ring-2 ring-background" />
          ) : null}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 p-0" sideOffset={8}>
        <div className="border-b border-border px-4 py-3">
          <p className="text-sm font-semibold text-foreground">Уведомления</p>
          <p className="text-xs text-muted-foreground">{subtitle}</p>
        </div>
        <div className="max-h-72 overflow-y-auto px-4 py-6">
          <p className="text-center text-sm text-muted-foreground">
            Пока нет новых уведомлений.
          </p>
        </div>
      </PopoverContent>
    </Popover>
  );
}
