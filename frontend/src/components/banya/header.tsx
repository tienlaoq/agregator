"use client"

import dynamic from "next/dynamic"
import { useState } from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from "@/components/ui/sheet"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import {
  Flame,
  Menu,
  User,
  CalendarDays,
  LogOut,
  Shield,
  Building2,
  Users,
  Activity,
  MessageSquare,
  LayoutGrid,
  Info,
  LifeBuoy,
  ChevronRight,
  Phone,
  Send,
  Clock,
  type LucideIcon,
} from "lucide-react"
import { VkIcon } from "@/components/banya/footer"
import { SITE_PHONE, SITE_PHONE_HREF, SUPPORT_HOURS, TELEGRAM_URL, VK_URL } from "@/lib/site-contacts"
import { useAuthStore } from "@/store/auth"

// Соц-доказательство. ponytail: числа — плейсхолдеры (как «500+» на главной);
// подставить реальные из аналитики отдельной задачей.
const PLATFORM_STATS: [string, string][] = [
  ["500+", "заведений"],
  ["40+", "городов"],
  ["24/7", "бронь"],
]

// Общий стиль строки меню: крупная тап-цель, скруглённый hover/active-фон.
const mobileRow =
  "flex items-center gap-3 rounded-xl px-3 py-3 text-base font-medium transition-colors hover:bg-secondary active:bg-secondary"

const PRIMARY_NAV: { href: string; label: string; Icon: LucideIcon }[] = [
  { href: "/venues", label: "Каталог", Icon: LayoutGrid },
  { href: "/masters", label: "Мастера", Icon: Users },
  { href: "/about", label: "О нас", Icon: Info },
  { href: "/support", label: "Поддержка", Icon: LifeBuoy },
]

const NotificationBell = dynamic(
  () =>
    import("@/components/banya/notification-bell").then((m) => ({
      default: m.NotificationBell,
    })),
  { ssr: false, loading: () => null },
)

interface HeaderProps {
  isLoggedIn?: boolean
  userName?: string
  userAvatar?: string
  userRole?: string
  /** True when the user manages ≥1 venue via CRM (owner or invited staff). */
  hasManagedVenues?: boolean
}

export function Header({ isLoggedIn = false, userName = "Иван", userAvatar, userRole, hasManagedVenues = false }: HeaderProps) {
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const router = useRouter()
  const logout = useAuthStore((s) => s.logout)

  const handleLogout = () => {
    logout()
    setMobileMenuOpen(false)
    router.push("/")
  }

  // Ссылки кабинета для мобильного меню — собираем по роли, чтобы не плодить
  // ветвистый JSX. Порядок = порядок отрисовки.
  const accountLinks: { href: string; label: string; Icon: LucideIcon }[] = []
  if (userRole === "admin") {
    accountLinks.push(
      { href: "/admin/venues", label: "Модерация заведений", Icon: Shield },
      { href: "/admin/masters", label: "Модерация мастеров", Icon: Users },
      { href: "/admin/metrics", label: "Метрики платформы", Icon: Activity },
      { href: "/admin/support", label: "Обращения в поддержку", Icon: MessageSquare },
    )
  } else if (userRole === "master") {
    accountLinks.push({ href: "/owner/master", label: "Кабинет мастера", Icon: Users })
  } else if (userRole === "venue_owner") {
    accountLinks.push({ href: "/owner/venues", label: "Мои заведения", Icon: Building2 })
  }
  if (hasManagedVenues && userRole !== "venue_owner") {
    accountLinks.push({ href: "/owner/venues", label: "Заведения команды", Icon: Building2 })
  }
  accountLinks.push(
    { href: "/my/bookings", label: "Мои бронирования", Icon: CalendarDays },
    { href: "/my", label: "Личный кабинет", Icon: User },
  )

  return (
    <header className="sticky top-0 z-50 w-full border-b border-border bg-background/95 pt-[env(safe-area-inset-top)] backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="container mx-auto flex h-16 items-center justify-between px-4">
        <Link href="/" className="flex items-center gap-2">
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary">
            <Flame className="h-5 w-5 text-primary-foreground" />
          </div>
          <span className="text-xl font-bold text-foreground">БаняГид</span>
        </Link>

        <nav className="hidden items-center gap-6 md:flex">
          <Link href="/venues" className="text-sm font-medium text-muted-foreground transition-colors hover:text-foreground">
            Каталог
          </Link>
          <Link href="/masters" className="text-sm font-medium text-muted-foreground transition-colors hover:text-foreground">
            Мастера
          </Link>
          <Link href="/about" className="text-sm font-medium text-muted-foreground transition-colors hover:text-foreground">
            О нас
          </Link>
          <Link href="/support" className="text-sm font-medium text-muted-foreground transition-colors hover:text-foreground">
            Поддержка
          </Link>
        </nav>

        <div className="hidden items-center gap-2 md:flex">
          <NotificationBell />
          {isLoggedIn ? (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" className="relative h-9 w-9 rounded-full">
                  <Avatar className="h-9 w-9">
                    <AvatarImage src={userAvatar} alt={userName} />
                    <AvatarFallback className="bg-primary text-primary-foreground">
                      {userName.charAt(0).toUpperCase()}
                    </AvatarFallback>
                  </Avatar>
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent className="w-56" align="end" forceMount>
                {userRole === "admin" ? (
                  <>
                    <DropdownMenuItem asChild>
                      <Link href="/admin/venues" className="flex items-center gap-2">
                        <Shield className="h-4 w-4" />
                        Модерация заведений
                      </Link>
                    </DropdownMenuItem>
                    <DropdownMenuItem asChild>
                      <Link href="/admin/masters" className="flex items-center gap-2">
                        <Users className="h-4 w-4" />
                        Модерация мастеров
                      </Link>
                    </DropdownMenuItem>
                    <DropdownMenuItem asChild>
                      <Link href="/admin/metrics" className="flex items-center gap-2">
                        <Activity className="h-4 w-4" />
                        Метрики платформы
                      </Link>
                    </DropdownMenuItem>
                    <DropdownMenuItem asChild>
                      <Link href="/admin/support" className="flex items-center gap-2">
                        <MessageSquare className="h-4 w-4" />
                        Обращения в поддержку
                      </Link>
                    </DropdownMenuItem>
                  </>
                ) : userRole === "master" ? (
                  <DropdownMenuItem asChild>
                    <Link href="/owner/master" className="flex items-center gap-2">
                      <Users className="h-4 w-4" />
                      Кабинет мастера
                    </Link>
                  </DropdownMenuItem>
                ) : userRole === "venue_owner" ? (
                  <DropdownMenuItem asChild>
                    <Link href="/owner/venues" className="flex items-center gap-2">
                      <Building2 className="h-4 w-4" />
                      Мои заведения
                    </Link>
                  </DropdownMenuItem>
                ) : null}
                {hasManagedVenues && userRole !== "venue_owner" ? (
                  <DropdownMenuItem asChild>
                    <Link href="/owner/venues" className="flex items-center gap-2">
                      <Building2 className="h-4 w-4" />
                      Заведения команды
                    </Link>
                  </DropdownMenuItem>
                ) : null}
                <DropdownMenuItem asChild>
                  <Link href="/my/bookings" className="flex items-center gap-2">
                    <CalendarDays className="h-4 w-4" />
                    Мои бронирования
                  </Link>
                </DropdownMenuItem>
                <DropdownMenuItem asChild>
                  <Link href="/my" className="flex items-center gap-2">
                    <User className="h-4 w-4" />
                    Личный кабинет
                  </Link>
                </DropdownMenuItem>
                <DropdownMenuItem asChild>
                  <Link href="/support" className="flex items-center gap-2">
                    <Users className="h-4 w-4" />
                    Поддержка
                  </Link>
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem className="text-destructive" onClick={handleLogout}>
                  <LogOut className="mr-2 h-4 w-4" />
                  Выйти
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          ) : (
            <>
              <Button variant="ghost" asChild>
                <Link href="/auth/login">Войти</Link>
              </Button>
              <Button asChild>
                <Link href="/auth/register">Регистрация</Link>
              </Button>
            </>
          )}
        </div>

        <div className="flex items-center gap-1 md:hidden">
          <NotificationBell />
          <Sheet open={mobileMenuOpen} onOpenChange={setMobileMenuOpen}>
            <SheetTrigger asChild>
              <Button variant="ghost" size="icon">
                <Menu className="h-5 w-5" />
                <span className="sr-only">Открыть меню</span>
              </Button>
            </SheetTrigger>
          <SheetContent side="right" className="flex w-[86%] max-w-sm flex-col gap-0 p-0">
            <SheetTitle className="sr-only">Меню</SheetTitle>

            {/* Скролл-область: safe-area сверху уводит контент из-под чёлки, оставляя место кнопке × */}
            <div className="flex-1 overflow-y-auto px-3 pb-4 pt-[calc(env(safe-area-inset-top)+3.5rem)]">
              <nav className="flex flex-col gap-1">
                {PRIMARY_NAV.map(({ href, label, Icon }) => (
                  <Link key={href} href={href} className={mobileRow} onClick={() => setMobileMenuOpen(false)}>
                    <Icon className="h-5 w-5 shrink-0 text-muted-foreground" />
                    {label}
                  </Link>
                ))}
              </nav>

              {isLoggedIn && (
                <>
                  <div className="mt-4 flex items-center gap-3 rounded-xl bg-secondary/50 px-3 py-3">
                    <Avatar className="h-11 w-11">
                      <AvatarImage src={userAvatar} alt={userName} />
                      <AvatarFallback className="bg-primary text-primary-foreground">
                        {userName.charAt(0).toUpperCase()}
                      </AvatarFallback>
                    </Avatar>
                    <span className="min-w-0 truncate font-semibold">{userName}</span>
                  </div>

                  <nav className="mt-2 flex flex-col gap-1">
                    {accountLinks.map(({ href, label, Icon }) => (
                      <Link
                        key={label}
                        href={href}
                        className={`${mobileRow} text-muted-foreground`}
                        onClick={() => setMobileMenuOpen(false)}
                      >
                        <Icon className="h-5 w-5 shrink-0" />
                        <span className="flex-1">{label}</span>
                        <ChevronRight className="h-4 w-4 shrink-0 opacity-40" />
                      </Link>
                    ))}
                  </nav>
                </>
              )}

              {/* Инфо о платформе — заполняет меню и возвращает контент скрытого
                  в приложении веб-футера (контакты/партнёрам/соцсети). */}
              <div className="mt-4 space-y-3 border-t border-border pt-4">
                <div className="grid grid-cols-3 gap-2">
                  {PLATFORM_STATS.map(([value, label]) => (
                    <div key={label} className="rounded-xl bg-secondary/60 px-2 py-2.5 text-center">
                      <div className="text-lg font-semibold text-primary">{value}</div>
                      <div className="text-[11px] text-muted-foreground">{label}</div>
                    </div>
                  ))}
                </div>

                <div className="rounded-xl border border-border bg-card px-3">
                  <a href={SITE_PHONE_HREF} className="flex items-center gap-2.5 py-2.5 text-sm">
                    <Phone className="h-4 w-4 shrink-0 text-primary" />
                    {SITE_PHONE}
                  </a>
                  <a
                    href={TELEGRAM_URL}
                    target="_blank"
                    rel="noreferrer"
                    className="flex items-center gap-2.5 border-t border-border py-2.5 text-sm"
                  >
                    <Send className="h-4 w-4 shrink-0 text-primary" />
                    @banyagid
                  </a>
                  <div className="flex items-center gap-2.5 border-t border-border py-2.5 text-sm text-muted-foreground">
                    <Clock className="h-4 w-4 shrink-0 text-primary" />
                    {SUPPORT_HOURS}
                  </div>
                </div>

                <Link
                  href="/partner"
                  onClick={() => setMobileMenuOpen(false)}
                  className="flex items-center justify-between gap-2 rounded-xl border border-primary/25 bg-card px-3 py-3 text-sm font-medium"
                >
                  <span className="flex items-center gap-2.5">
                    <Building2 className="h-[18px] w-[18px] shrink-0 text-primary" />
                    Разместить заведение
                  </span>
                  <ChevronRight className="h-4 w-4 shrink-0 text-primary" />
                </Link>

                <div className="grid grid-cols-2 gap-2">
                  <a
                    href={TELEGRAM_URL}
                    target="_blank"
                    rel="noreferrer"
                    className="flex items-center justify-center gap-2 rounded-xl bg-secondary/60 py-2.5 text-sm font-medium"
                  >
                    <Send className="h-4 w-4" />
                    Telegram
                  </a>
                  <a
                    href={VK_URL}
                    target="_blank"
                    rel="noreferrer"
                    className="flex items-center justify-center gap-2 rounded-xl bg-secondary/60 py-2.5 text-sm font-medium"
                  >
                    <VkIcon className="h-4 w-4" />
                    ВКонтакте
                  </a>
                </div>
              </div>
            </div>

            {/* Закреплённый низ: safe-area снизу уводит кнопку от home-indicator */}
            <div className="border-t border-border px-3 py-4 pb-[calc(env(safe-area-inset-bottom)+1rem)]">
              {isLoggedIn ? (
                <Button variant="destructive" className="w-full" onClick={handleLogout}>
                  <LogOut className="mr-2 h-4 w-4" />
                  Выйти
                </Button>
              ) : (
                <div className="flex flex-col gap-2">
                  <Button variant="outline" asChild className="w-full">
                    <Link href="/auth/login" onClick={() => setMobileMenuOpen(false)}>Войти</Link>
                  </Button>
                  <Button asChild className="w-full">
                    <Link href="/auth/register" onClick={() => setMobileMenuOpen(false)}>Регистрация</Link>
                  </Button>
                </div>
              )}

              <div className="mt-3 flex items-center justify-center gap-2 text-[11px] text-muted-foreground">
                <Link href="/privacy" onClick={() => setMobileMenuOpen(false)}>Политика</Link>
                <span aria-hidden>·</span>
                <Link href="/consent" onClick={() => setMobileMenuOpen(false)}>Согласие</Link>
              </div>
            </div>
          </SheetContent>
          </Sheet>
        </div>
      </div>
    </header>
  )
}
