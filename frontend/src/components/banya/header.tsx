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
import { Sheet, SheetContent, SheetTrigger } from "@/components/ui/sheet"
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
} from "lucide-react"
import { useAuthStore } from "@/store/auth"

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
}

export function Header({ isLoggedIn = false, userName = "Иван", userAvatar, userRole }: HeaderProps) {
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const router = useRouter()
  const logout = useAuthStore((s) => s.logout)

  const handleLogout = () => {
    logout()
    setMobileMenuOpen(false)
    router.push("/")
  }

  return (
    <header className="sticky top-0 z-50 w-full border-b border-border bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
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
                <DropdownMenuItem asChild>
                  <Link href="/my/bookings" className="flex items-center gap-2">
                    <CalendarDays className="h-4 w-4" />
                    Мои бронирования
                  </Link>
                </DropdownMenuItem>
                <DropdownMenuItem asChild>
                  <Link href="/my/profile" className="flex items-center gap-2">
                    <User className="h-4 w-4" />
                    Профиль
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
          <SheetContent side="right" className="w-[300px]">
            <div className="flex flex-col gap-6 pt-6">
              <Link
                href="/venues"
                className="text-lg font-medium"
                onClick={() => setMobileMenuOpen(false)}
              >
                Каталог
              </Link>
              <Link
                href="/masters"
                className="text-lg font-medium"
                onClick={() => setMobileMenuOpen(false)}
              >
                Мастера
              </Link>
              <Link
                href="/about"
                className="text-lg font-medium"
                onClick={() => setMobileMenuOpen(false)}
              >
                О нас
              </Link>
              <Link
                href="/support"
                className="text-lg font-medium"
                onClick={() => setMobileMenuOpen(false)}
              >
                Поддержка
              </Link>
              <div className="border-t border-border pt-6">
                {isLoggedIn ? (
                  <div className="flex flex-col gap-4">
                    <div className="flex items-center gap-3">
                      <Avatar className="h-10 w-10">
                        <AvatarImage src={userAvatar} alt={userName} />
                        <AvatarFallback className="bg-primary text-primary-foreground">
                          {userName.charAt(0).toUpperCase()}
                        </AvatarFallback>
                      </Avatar>
                      <span className="font-medium">{userName}</span>
                    </div>
                    {userRole === "admin" ? (
                      <>
                        <Link
                          href="/admin/venues"
                          className="flex items-center gap-2 text-muted-foreground"
                          onClick={() => setMobileMenuOpen(false)}
                        >
                          <Shield className="h-4 w-4 shrink-0" />
                          Модерация заведений
                        </Link>
                        <Link
                          href="/admin/masters"
                          className="flex items-center gap-2 text-muted-foreground"
                          onClick={() => setMobileMenuOpen(false)}
                        >
                          <Users className="h-4 w-4 shrink-0" />
                          Модерация мастеров
                        </Link>
                        <Link
                          href="/admin/metrics"
                          className="flex items-center gap-2 text-muted-foreground"
                          onClick={() => setMobileMenuOpen(false)}
                        >
                          <Activity className="h-4 w-4 shrink-0" />
                          Метрики платформы
                        </Link>
                        <Link
                          href="/admin/support"
                          className="flex items-center gap-2 text-muted-foreground"
                          onClick={() => setMobileMenuOpen(false)}
                        >
                          <MessageSquare className="h-4 w-4 shrink-0" />
                          Обращения в поддержку
                        </Link>
                      </>
                    ) : userRole === "master" ? (
                      <Link
                        href="/owner/master"
                        className="flex items-center gap-2 text-muted-foreground"
                        onClick={() => setMobileMenuOpen(false)}
                      >
                        <Users className="h-4 w-4 shrink-0" />
                        Кабинет мастера
                      </Link>
                    ) : userRole === "venue_owner" ? (
                      <Link
                        href="/owner/venues"
                        className="flex items-center gap-2 text-muted-foreground"
                        onClick={() => setMobileMenuOpen(false)}
                      >
                        <Building2 className="h-4 w-4 shrink-0" />
                        Мои заведения
                      </Link>
                    ) : null}
                    <Link href="/my/bookings" className="text-muted-foreground" onClick={() => setMobileMenuOpen(false)}>
                      Мои бронирования
                    </Link>
                    <Link href="/my/profile" className="text-muted-foreground" onClick={() => setMobileMenuOpen(false)}>
                      Профиль
                    </Link>
                    <Link href="/support" className="text-muted-foreground" onClick={() => setMobileMenuOpen(false)}>
                      Поддержка
                    </Link>
                    <Button variant="destructive" className="mt-2" onClick={handleLogout}>
                      <LogOut className="mr-2 h-4 w-4" />
                      Выйти
                    </Button>
                  </div>
                ) : (
                  <div className="flex flex-col gap-3">
                    <Button variant="outline" asChild className="w-full">
                      <Link href="/auth/login" onClick={() => setMobileMenuOpen(false)}>Войти</Link>
                    </Button>
                    <Button asChild className="w-full">
                      <Link href="/auth/register" onClick={() => setMobileMenuOpen(false)}>Регистрация</Link>
                    </Button>
                  </div>
                )}
              </div>
            </div>
          </SheetContent>
          </Sheet>
        </div>
      </div>
    </header>
  )
}
