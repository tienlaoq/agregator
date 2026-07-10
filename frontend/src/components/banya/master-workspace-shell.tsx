"use client";

import type { ReactNode } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { getMyMasterProfile } from "@/lib/api";
import { MASTER_PROFILE_STATUS_LABELS } from "@/lib/types";
import { useAuthStore } from "@/store/auth";
import {
  LayoutDashboard,
  CalendarClock,
  BookMarked,
  Users,
  ListChecks,
  Wallet,
  CalendarPlus,
  Heart,
  BarChart3,
  Star,
  UserCircle,
  Flame,
  type LucideIcon,
} from "lucide-react";

type NavItem = {
  label: string;
  icon: LucideIcon;
  /** relative to /owner/master; "" = Сегодня (index) */
  sub: string;
  /** exact-match highlighting for the index and section roots that share a prefix */
  exact?: boolean;
  /** page not built yet — shown disabled with a "Скоро" chip */
  soon?: boolean;
  /** paid module — shown with a "Про" chip, not navigable in the free tier */
  pro?: boolean;
};

type NavGroup = { title: string; items: NavItem[] };

const NAV: NavGroup[] = [
  {
    title: "Операции",
    items: [
      { label: "Сегодня", icon: LayoutDashboard, sub: "", exact: true },
      { label: "Расписание", icon: CalendarClock, sub: "/schedule" },
      { label: "Заявки", icon: BookMarked, sub: "/bookings" },
      { label: "Клиенты", icon: Users, sub: "/clients" },
      { label: "Услуги и цены", icon: ListChecks, sub: "/services", soon: true },
    ],
  },
  {
    title: "Деньги",
    items: [{ label: "Финансы · выплаты", icon: Wallet, sub: "/finance" }],
  },
  {
    title: "Рост · Про",
    items: [
      { label: "Онлайн-запись", icon: CalendarPlus, sub: "/online", pro: true },
      { label: "Возврат клиентов", icon: Heart, sub: "/retention", pro: true },
      { label: "Аналитика загрузки", icon: BarChart3, sub: "/analytics", pro: true },
    ],
  },
  {
    title: "",
    items: [
      { label: "Отзывы", icon: Star, sub: "/reviews", soon: true },
      { label: "Профиль мастера", icon: UserCircle, sub: "/profile" },
    ],
  },
];

const BASE = "/owner/master";

export function MasterWorkspaceShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const { token, user } = useAuthStore();

  const { data: profile } = useQuery({
    queryKey: ["my-master-profile"],
    queryFn: getMyMasterProfile,
    enabled: !!token && user?.role === "master",
    retry: false,
  });

  const statusMeta = profile
    ? MASTER_PROFILE_STATUS_LABELS[profile.status]
    : null;
  const isActive = profile?.status === "active";

  function itemActive(item: NavItem): boolean {
    const href = BASE + item.sub;
    return item.exact ? pathname === href : pathname.startsWith(href);
  }

  return (
    <div className="flex min-h-[calc(100vh-4rem)]">
      <aside className="hidden w-56 shrink-0 flex-col gap-1 border-r border-border bg-muted/20 p-3 md:flex">
        <div className="mb-2 flex items-center gap-2 rounded-lg border border-border bg-background px-2 py-2">
          <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-primary/10">
            <Flame className="h-4 w-4 text-primary" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium">
              {profile?.display_name || "Мастер"}
            </div>
            <div
              className={`text-[11px] ${
                isActive ? "text-emerald-600" : "text-muted-foreground"
              }`}
            >
              {statusMeta?.label ?? "профиль"}
            </div>
          </div>
        </div>

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
              const active = itemActive(item);
              const cls = `flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm ${
                active
                  ? "bg-primary/10 font-medium text-primary"
                  : "text-muted-foreground hover:bg-muted"
              }`;
              const inner = (
                <>
                  <item.icon className="h-4 w-4 shrink-0" />
                  <span className="flex-1 truncate">{item.label}</span>
                  {item.pro ? (
                    <span className="rounded-full bg-violet-500/10 px-1.5 text-[11px] text-violet-600 dark:text-violet-300">
                      Про
                    </span>
                  ) : item.soon ? (
                    <span className="rounded-full bg-muted px-1.5 text-[11px] text-muted-foreground">
                      Скоро
                    </span>
                  ) : null}
                </>
              );
              if (item.pro || item.soon) {
                return (
                  <div
                    key={item.label}
                    className={`${cls} cursor-not-allowed opacity-70`}
                    title={
                      item.pro ? "Доступно на тарифе Про" : "Скоро в кабинете"
                    }
                  >
                    {inner}
                  </div>
                );
              }
              return (
                <Link key={item.label} href={BASE + item.sub} className={cls}>
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
