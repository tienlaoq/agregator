"use client";

import type { ReactNode } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { getOwnerVenues } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import {
  LayoutDashboard,
  CalendarClock,
  BookMarked,
  Users,
  ClipboardList,
  Wallet,
  Package,
  Filter,
  Send,
  BarChart3,
  Star,
  Building2,
  Lock,
  type LucideIcon,
} from "lucide-react";

type NavItem = {
  label: string;
  icon: LucideIcon;
  /** relative to /owner/venues/{venueId}; "" = Сегодня (index) */
  sub: string;
  /** exact-match highlighting (index and section roots that share a prefix) */
  exact?: boolean;
  /** paid module — shown with a lock, not navigable in the free tier */
  locked?: boolean;
  badge?: string;
};

type NavGroup = { title: string; items: NavItem[] };

const NAV: NavGroup[] = [
  {
    title: "Операции",
    items: [
      { label: "Сегодня", icon: LayoutDashboard, sub: "", exact: true },
      { label: "Расписание", icon: CalendarClock, sub: "/crm/schedule" },
      { label: "Брони", icon: BookMarked, sub: "/bookings" },
      { label: "Гости", icon: Users, sub: "/crm/guests" },
      { label: "Задачи · команда", icon: ClipboardList, sub: "/crm", exact: true },
    ],
  },
  {
    title: "Деньги и запасы",
    items: [
      { label: "Финансы · касса", icon: Wallet, sub: "/finance" },
      { label: "Склад · товары", icon: Package, sub: "/stock" },
    ],
  },
  {
    title: "Рост · тариф Про",
    items: [
      { label: "Лиды · воронка", icon: Filter, sub: "/leads", locked: true },
      { label: "Рассылки", icon: Send, sub: "/mailing", locked: true },
      { label: "Аналитика", icon: BarChart3, sub: "/analytics", locked: true },
    ],
  },
  {
    title: "",
    items: [
      { label: "Отзывы", icon: Star, sub: "/reviews" },
      { label: "Заведение", icon: Building2, sub: "/edit" },
    ],
  },
];

export function OwnerWorkspaceShell({
  venueId,
  children,
}: {
  venueId: string;
  children: ReactNode;
}) {
  const pathname = usePathname();
  const { token } = useAuthStore();

  const { data: venues } = useQuery({
    queryKey: ["owner-venues"],
    queryFn: getOwnerVenues,
    enabled: !!token,
  });
  const venue = venues?.find((v) => v.id === venueId) ?? null;
  const base = `/owner/venues/${venueId}`;

  function isActive(item: NavItem): boolean {
    const href = base + item.sub;
    return item.exact ? pathname === href : pathname.startsWith(href);
  }

  return (
    <div className="flex min-h-[calc(100vh-4rem)]">
      <aside className="hidden w-56 shrink-0 flex-col gap-1 border-r border-border bg-muted/20 p-3 md:flex">
        {venues && venues.length > 1 ? (
          <div className="mb-2">
            <Select
              value={venueId}
              onValueChange={(id) => {
                window.location.href = `/owner/venues/${id}`;
              }}
            >
              <SelectTrigger className="h-9 w-full text-sm">
                <SelectValue placeholder="Заведение" />
              </SelectTrigger>
              <SelectContent>
                {venues.map((v) => (
                  <SelectItem key={v.id} value={v.id}>
                    {v.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        ) : (
          <div className="mb-2 flex items-center gap-2 px-2 py-1">
            <Building2 className="h-4 w-4 text-muted-foreground" />
            <span className="truncate text-sm font-medium">
              {venue?.name ?? "Заведение"}
            </span>
          </div>
        )}

        {NAV.map((group) => (
          <div key={group.title || "root"} className="flex flex-col gap-0.5">
            {group.title ? (
              <div className="px-2 pb-1 pt-3 text-[11px] uppercase tracking-wide text-muted-foreground/70">
                {group.title}
              </div>
            ) : (
              <div className="my-1 border-t border-border" />
            )}
            {group.items.map((item) => {
              const active = isActive(item);
              const cls = `flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm ${
                active
                  ? "bg-primary/10 font-medium text-primary"
                  : "text-muted-foreground hover:bg-muted"
              }`;
              const inner = (
                <>
                  <item.icon className="h-4 w-4 shrink-0" />
                  <span className="flex-1 truncate">{item.label}</span>
                  {item.locked ? (
                    <Lock className="h-3.5 w-3.5 text-muted-foreground/60" />
                  ) : item.badge ? (
                    <span className="rounded-full bg-destructive/10 px-1.5 text-[11px] text-destructive">
                      {item.badge}
                    </span>
                  ) : null}
                </>
              );
              if (item.locked) {
                return (
                  <div
                    key={item.label}
                    className={`${cls} cursor-not-allowed opacity-70`}
                    title="Доступно на тарифе Про"
                  >
                    {inner}
                  </div>
                );
              }
              return (
                <Link key={item.label} href={base + item.sub} className={cls}>
                  {inner}
                </Link>
              );
            })}
          </div>
        ))}
      </aside>

      <div className="min-w-0 flex-1">{children}</div>
    </div>
  );
}
