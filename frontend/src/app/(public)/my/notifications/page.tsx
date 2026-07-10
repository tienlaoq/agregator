"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { useAuthStore } from "@/store/auth";
import { useNotifications } from "@/features/notifications/hooks/use-notifications";
import type { Notification } from "@/features/notifications/types/notification";
import { Bell, CheckCheck } from "lucide-react";
import { cn } from "@/lib/utils";

/** «5 минут назад» / «вчера» — короткая относительная метка. */
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
  return new Date(then).toLocaleDateString("ru-RU", { day: "numeric", month: "short" });
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
        "flex w-full gap-3 rounded-xl border border-border px-4 py-3 text-left transition-colors",
        item.read ? "hover:bg-muted/50" : "bg-primary/5 hover:bg-primary/10",
      )}
    >
      <span
        className={cn(
          "mt-1.5 h-2 w-2 shrink-0 rounded-full",
          item.read ? "bg-transparent" : "bg-primary",
        )}
        aria-hidden
      />
      <div className="min-w-0 flex-1">
        <p className="text-sm font-semibold leading-snug">{item.title}</p>
        {item.body ? (
          <p className="mt-0.5 text-sm leading-snug text-muted-foreground">{item.body}</p>
        ) : null}
        {item.created_at ? (
          <p className="mt-1 text-xs text-muted-foreground/70">
            {formatRelative(item.created_at)}
          </p>
        ) : null}
      </div>
    </button>
  );
}

export default function MyNotificationsPage() {
  const router = useRouter();
  const { token, hydrated } = useAuthStore();

  useEffect(() => {
    if (hydrated && !token) router.push("/auth/login");
  }, [hydrated, token, router]);

  const { list, unreadCount, isLoading, markRead, markAllRead, isMarkingAll } =
    useNotifications(Boolean(hydrated && token));

  if (!hydrated || !token) return null;

  return (
    <div className="mx-auto max-w-3xl px-4 py-8 md:px-8">
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-bold">Уведомления</h1>
        {unreadCount > 0 && (
          <Button
            variant="outline"
            size="sm"
            className="gap-2"
            onClick={() => markAllRead()}
            disabled={isMarkingAll}
          >
            <CheckCheck className="h-4 w-4" />
            {isMarkingAll ? "Отмечаем…" : "Прочитать все"}
          </Button>
        )}
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="h-16 animate-pulse rounded-xl bg-muted" />
          ))}
        </div>
      ) : list.length > 0 ? (
        <div className="space-y-2">
          {list.map((item) => (
            <NotificationRow key={item.id} item={item} onRead={markRead} />
          ))}
        </div>
      ) : (
        <div className="flex flex-col items-center py-16 text-center">
          <Bell className="mb-4 h-10 w-10 text-muted-foreground/50" />
          <h3 className="mb-2 text-lg font-semibold">Пока нет уведомлений</h3>
          <p className="text-sm text-muted-foreground">
            Здесь появятся статусы броней и сообщения от заведений
          </p>
        </div>
      )}
    </div>
  );
}
