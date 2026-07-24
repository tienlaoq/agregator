"use client";

import { useState, type ReactNode } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
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
  Share2,
  Star,
  Building2,
  Plus,
  Check,
  ChevronsUpDown,
  Menu,
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
      { label: "Аналитика", icon: BarChart3, sub: "/analytics" },
      { label: "Расписание", icon: CalendarClock, sub: "/crm/schedule" },
      { label: "Брони", icon: BookMarked, sub: "/bookings" },
      { label: "Гости", icon: Users, sub: "/crm/guests" },
      { label: "Виджет брони", icon: Share2, sub: "/share" },
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
  const [mobileNavOpen, setMobileNavOpen] = useState(false);

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

  // Переключатель заведений — общий для десктоп-сайдбара и мобильного дровера.
  const venueSwitcher = (onNavigate?: () => void) => (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-sm hover:bg-muted"
        >
          <Building2 className="h-4 w-4 shrink-0 text-muted-foreground" />
          <span className="flex-1 truncate text-left font-medium">
            {venue?.name ?? "Заведение"}
          </span>
          <ChevronsUpDown className="h-4 w-4 shrink-0 text-muted-foreground" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56">
        <DropdownMenuLabel>Мои заведения</DropdownMenuLabel>
        {(venues ?? []).map((v) => (
          <DropdownMenuItem key={v.id} asChild>
            <Link href={`/owner/venues/${v.id}`} className="cursor-pointer" onClick={onNavigate}>
              <Check
                className={`mr-2 h-4 w-4 ${
                  v.id === venueId ? "opacity-100" : "opacity-0"
                }`}
              />
              <span className="truncate">{v.name}</span>
            </Link>
          </DropdownMenuItem>
        ))}
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <Link href="/owner/venues/new" className="cursor-pointer" onClick={onNavigate}>
            <Plus className="mr-2 h-4 w-4" />
            Создать заведение
          </Link>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );

  // Группы навигации — общие для сайдбара и дровера. onNavigate закрывает дровер.
  const navGroups = (onNavigate?: () => void) =>
    NAV.map((group) => (
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
                <span className="rounded-full bg-muted px-1.5 text-[11px] text-muted-foreground">
                  Скоро
                </span>
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
            <Link key={item.label} href={base + item.sub} className={cls} onClick={onNavigate}>
              {inner}
            </Link>
          );
        })}
      </div>
    ));

  return (
    <div className="flex min-h-[calc(100vh-4rem)]">
      <aside className="hidden w-56 shrink-0 flex-col gap-1 border-r border-border bg-muted/20 p-3 md:flex">
        <div className="mb-2">{venueSwitcher()}</div>
        {navGroups()}
      </aside>

      <div className="min-w-0 flex-1">
        {/* Мобильная навигация: сайдбар скрыт на <md, иначе кабинет остаётся
            без навигации и владелец залипает на «Сегодня». Дровер держит те же
            разделы. Липнет под AppHeader (safe-area + h-16). */}
        <div className="sticky top-[calc(env(safe-area-inset-top)+4rem)] z-30 flex items-center gap-2 border-b border-border bg-background/95 px-3 py-2 backdrop-blur md:hidden">
          <Sheet open={mobileNavOpen} onOpenChange={setMobileNavOpen}>
            <SheetTrigger asChild>
              <button
                type="button"
                aria-label="Разделы кабинета"
                className="inline-flex shrink-0 items-center gap-2 rounded-lg border border-border px-3 py-1.5 text-sm font-medium"
              >
                <Menu className="h-4 w-4" />
                Разделы
              </button>
            </SheetTrigger>
            <SheetContent
              side="left"
              className="w-[280px] overflow-y-auto p-3 pt-[calc(env(safe-area-inset-top)+3.5rem)]"
            >
              <SheetTitle className="sr-only">Навигация по кабинету</SheetTitle>
              <div className="mb-2">{venueSwitcher(() => setMobileNavOpen(false))}</div>
              <div className="flex flex-col gap-1">{navGroups(() => setMobileNavOpen(false))}</div>
            </SheetContent>
          </Sheet>
          <span className="truncate text-sm font-medium">{venue?.name ?? "Заведение"}</span>
        </div>

        {children}
      </div>
    </div>
  );
}
