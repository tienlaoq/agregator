"use client";

import { Bell, CheckCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { useAuthStore, hasAuthSession } from "@/store/auth";
import { useNotifications } from "@/features/notifications/hooks/use-notifications";
import type { Notification } from "@/features/notifications/types/notification";
import { cn } from "@/lib/utils";

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

/** «5 минут назад» / «вчера» — короткая относительная метка для ленты. */
function formatRelative(iso: string | undefined): string {
  if (!iso) return "";
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "";
  const diffSec = Math.round((Date.now() - then) / 1000);
  if (diffSec < 60) return "только что";
  const diffMin = Math.round(diffSec / 60);
  if (diffMin < 60) return `${diffMin} мин назад`;
  const diffHour = Math.round(diffMin / 60);
  if (diffHour < 24) return `${diffHour} ч назад`;
  const diffDay = Math.round(diffHour / 24);
  if (diffDay === 1) return "вчера";
  if (diffDay < 7) return `${diffDay} дн назад`;
  return new Date(then).toLocaleDateString("ru-RU", {
    day: "numeric",
    month: "short",
  });
}

function NotificationRow({
  item,
  onRead,
}: {
  item: Notification;
  onRead: (id: string) => void;
}) {
  return (
    <button
      type="button"
      onClick={() => {
        if (!item.read) onRead(item.id);
      }}
      className={cn(
        "flex w-full flex-col gap-0.5 rounded-lg px-3 py-2.5 text-left transition-colors",
        item.read ? "hover:bg-muted/50" : "bg-accent/40 hover:bg-accent/60",
      )}
    >
      <div className="flex items-start gap-2">
        {!item.read ? (
          <span
            className="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-destructive"
            aria-hidden
          />
        ) : (
          <span className="mt-1.5 h-2 w-2 shrink-0" aria-hidden />
        )}
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold leading-snug text-foreground">
            {item.title}
          </p>
          {item.body ? (
            <p className="mt-0.5 text-xs leading-snug text-muted-foreground">
              {item.body}
            </p>
          ) : null}
          {item.created_at ? (
            <p className="mt-1 text-[11px] text-muted-foreground/70">
              {formatRelative(item.created_at)}
            </p>
          ) : null}
        </div>
      </div>
    </button>
  );
}

/**
 * Виден только при живой сессии (токен + пользователь с id в сторе).
 * Показывает счётчик непрочитанных и ленту уведомлений из API; новые
 * уведомления приходят realtime по WebSocket.
 */
export function NotificationBell() {
  const user = useAuthStore((s) => s.user);
  const token = useAuthStore((s) => s.token);
  const hydrated = useAuthStore((s) => s.hydrated);

  const loggedIn = hasAuthSession(user, token);
  const { list, unreadCount, isLoading, markRead, markAllRead, isMarkingAll } =
    useNotifications(Boolean(hydrated && loggedIn));

  if (!hydrated || !loggedIn) return null;

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
          aria-label={
            hasUnread ? `Уведомления: непрочитанных ${unreadCount}` : "Уведомления"
          }
        >
          <Bell className="h-5 w-5" />
          {hasUnread ? (
            <span
              className="pointer-events-none absolute -right-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-destructive px-1 text-[10px] font-semibold leading-none text-destructive-foreground ring-2 ring-background"
              aria-hidden
            >
              {unreadCount > 99 ? "99+" : unreadCount}
            </span>
          ) : null}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 p-0" sideOffset={8}>
        <div className="flex items-start justify-between gap-2 border-b border-border px-4 py-3">
          <div className="min-w-0">
            <p className="text-sm font-semibold text-foreground">Уведомления</p>
            {subtitle ? (
              <p className="text-xs text-muted-foreground">{subtitle}</p>
            ) : null}
          </div>
          {hasUnread ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-7 shrink-0 gap-1 px-2 text-xs text-muted-foreground hover:text-foreground"
              onClick={() => markAllRead()}
              disabled={isMarkingAll}
            >
              <CheckCheck className="h-3.5 w-3.5" />
              Прочитать всё
            </Button>
          ) : null}
        </div>
        <div className="max-h-80 overflow-y-auto p-1.5">
          {isLoading ? (
            <p className="px-3 py-6 text-center text-sm text-muted-foreground">
              Загрузка…
            </p>
          ) : list.length === 0 ? (
            <p className="px-3 py-6 text-center text-sm text-muted-foreground">
              Пока нет новых уведомлений.
            </p>
          ) : (
            <div className="space-y-0.5">
              {list.map((item) => (
                <NotificationRow key={item.id} item={item} onRead={markRead} />
              ))}
            </div>
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}
