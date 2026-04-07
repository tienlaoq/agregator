"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { useAuthStore } from "@/store/auth";
import { Menu, X, User, LogOut, Building2 } from "lucide-react";

export function Header() {
  const { user, hydrated, logout } = useAuthStore();
  const pathname = usePathname();
  const router = useRouter();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [profileOpen, setProfileOpen] = useState(false);

  const handleLogout = () => {
    logout();
    setProfileOpen(false);
    router.push("/");
  };

  const navLinks = [
    { href: "/venues", label: "Каталог" },
    ...(user
      ? [{ href: "/my/bookings", label: "Мои бронирования" }]
      : []),
    ...(user?.role === "owner"
      ? [{ href: "/owner/venues", label: "Мои заведения" }]
      : []),
  ];

  return (
    <header className="sticky top-0 z-50 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="mx-auto flex h-14 max-w-7xl items-center justify-between px-4">
        <div className="flex items-center gap-6">
          <Link href="/" className="flex items-center gap-2">
            <span className="text-xl font-bold text-primary">🔥 БаняГид</span>
          </Link>
          <nav className="hidden items-center gap-4 md:flex">
            {navLinks.map((link) => (
              <Link
                key={link.href}
                href={link.href}
                className={`text-sm font-medium transition-colors hover:text-primary ${
                  pathname === link.href
                    ? "text-primary"
                    : "text-muted-foreground"
                }`}
              >
                {link.label}
              </Link>
            ))}
          </nav>
        </div>

        <div className="hidden items-center gap-2 md:flex">
          {hydrated && !user && (
            <>
              <Button variant="ghost" size="sm" render={<Link href="/auth/login" />}>
                Войти
              </Button>
              <Button size="sm" render={<Link href="/auth/register" />}>
                Регистрация
              </Button>
            </>
          )}
          {hydrated && user && (
            <div className="relative">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setProfileOpen(!profileOpen)}
              >
                <User className="mr-1 h-4 w-4" />
                {user.name}
              </Button>
              {profileOpen && (
                <div className="absolute right-0 top-full mt-1 w-48 rounded-lg border bg-popover p-1 shadow-lg">
                  <Link
                    href="/my/profile"
                    className="flex items-center gap-2 rounded-md px-3 py-2 text-sm hover:bg-accent"
                    onClick={() => setProfileOpen(false)}
                  >
                    <User className="h-4 w-4" />
                    Профиль
                  </Link>
                  <Link
                    href="/my/bookings"
                    className="flex items-center gap-2 rounded-md px-3 py-2 text-sm hover:bg-accent"
                    onClick={() => setProfileOpen(false)}
                  >
                    <Building2 className="h-4 w-4" />
                    Мои бронирования
                  </Link>
                  <button
                    onClick={handleLogout}
                    className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-destructive hover:bg-accent"
                  >
                    <LogOut className="h-4 w-4" />
                    Выйти
                  </button>
                </div>
              )}
            </div>
          )}
        </div>

        <Button
          variant="ghost"
          size="icon"
          className="md:hidden"
          onClick={() => setMobileOpen(!mobileOpen)}
        >
          {mobileOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
        </Button>
      </div>

      {mobileOpen && (
        <div className="border-t px-4 pb-4 pt-2 md:hidden">
          <nav className="flex flex-col gap-2">
            {navLinks.map((link) => (
              <Link
                key={link.href}
                href={link.href}
                className={`rounded-md px-3 py-2 text-sm font-medium transition-colors hover:bg-accent ${
                  pathname === link.href
                    ? "bg-accent text-primary"
                    : "text-muted-foreground"
                }`}
                onClick={() => setMobileOpen(false)}
              >
                {link.label}
              </Link>
            ))}
            {hydrated && !user && (
              <>
                <Link
                  href="/auth/login"
                  className="rounded-md px-3 py-2 text-sm font-medium text-muted-foreground hover:bg-accent"
                  onClick={() => setMobileOpen(false)}
                >
                  Войти
                </Link>
                <Link
                  href="/auth/register"
                  className="rounded-md bg-primary px-3 py-2 text-center text-sm font-medium text-primary-foreground"
                  onClick={() => setMobileOpen(false)}
                >
                  Регистрация
                </Link>
              </>
            )}
            {hydrated && user && (
              <button
                onClick={handleLogout}
                className="rounded-md px-3 py-2 text-left text-sm font-medium text-destructive hover:bg-accent"
              >
                Выйти
              </button>
            )}
          </nav>
        </div>
      )}
    </header>
  );
}
