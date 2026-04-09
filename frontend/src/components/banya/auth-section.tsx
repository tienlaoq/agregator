"use client"

import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import Link from "next/link"
import { Flame, User, Building2 } from "lucide-react"

export function AuthSection() {
  const [role, setRole] = useState("user")

  return (
    <section id="auth" className="bg-secondary/30 py-16 md:py-24">
      <div className="container mx-auto px-4">
        <h2 className="mb-12 text-center text-3xl font-bold text-foreground md:text-4xl">
          Вход и регистрация
        </h2>

        <div className="grid gap-8 md:grid-cols-2 lg:gap-16">
          {/* Login Card */}
          <div id="login" className="flex justify-center">
            <Card className="w-full max-w-md border-border">
              <CardHeader className="text-center">
                <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary">
                  <Flame className="h-6 w-6 text-primary-foreground" />
                </div>
                <CardTitle className="text-2xl text-card-foreground">Вход</CardTitle>
                <CardDescription>Войдите в свой аккаунт БаняГид</CardDescription>
              </CardHeader>
              <CardContent>
                <form className="space-y-4">
                  <div className="space-y-2">
                    <Label htmlFor="login-email">Email</Label>
                    <Input
                      id="login-email"
                      type="email"
                      placeholder="your@email.com"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="login-password">Пароль</Label>
                    <Input
                      id="login-password"
                      type="password"
                      placeholder="••••••••"
                    />
                  </div>
                  <Button type="submit" className="w-full">
                    Войти
                  </Button>
                </form>
                <p className="mt-6 text-center text-sm text-muted-foreground">
                  Нет аккаунта?{" "}
                  <Link href="#register" className="font-medium text-primary hover:underline">
                    Зарегистрироваться
                  </Link>
                </p>
              </CardContent>
            </Card>
          </div>

          {/* Register Card */}
          <div id="register" className="flex justify-center">
            <Card className="w-full max-w-md border-border">
              <CardHeader className="text-center">
                <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary">
                  <Flame className="h-6 w-6 text-primary-foreground" />
                </div>
                <CardTitle className="text-2xl text-card-foreground">Регистрация</CardTitle>
                <CardDescription>Создайте аккаунт на БаняГид</CardDescription>
              </CardHeader>
              <CardContent>
                <form className="space-y-4">
                  <div className="space-y-2">
                    <Label htmlFor="register-name">Имя</Label>
                    <Input
                      id="register-name"
                      type="text"
                      placeholder="Иван Иванов"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="register-email">Email</Label>
                    <Input
                      id="register-email"
                      type="email"
                      placeholder="your@email.com"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="register-phone">Телефон</Label>
                    <Input
                      id="register-phone"
                      type="tel"
                      placeholder="+7 (999) 123-45-67"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="register-password">Пароль</Label>
                    <Input
                      id="register-password"
                      type="password"
                      placeholder="••••••••"
                    />
                  </div>

                  {/* Role Toggle */}
                  <div className="space-y-2">
                    <Label>Я регистрируюсь как</Label>
                    <ToggleGroup
                      type="single"
                      value={role}
                      onValueChange={(value) => value && setRole(value)}
                      className="grid grid-cols-2"
                    >
                      <ToggleGroupItem
                        value="user"
                        className="flex items-center gap-2 data-[state=on]:bg-primary data-[state=on]:text-primary-foreground"
                      >
                        <User className="h-4 w-4" />
                        Посетитель
                      </ToggleGroupItem>
                      <ToggleGroupItem
                        value="venue_owner"
                        className="flex items-center gap-2 data-[state=on]:bg-primary data-[state=on]:text-primary-foreground"
                      >
                        <Building2 className="h-4 w-4" />
                        Владелец бани
                      </ToggleGroupItem>
                    </ToggleGroup>
                  </div>

                  <Button type="submit" className="w-full">
                    Зарегистрироваться
                  </Button>
                </form>
                <p className="mt-6 text-center text-sm text-muted-foreground">
                  Уже есть аккаунт?{" "}
                  <Link href="#login" className="font-medium text-primary hover:underline">
                    Войти
                  </Link>
                </p>
              </CardContent>
            </Card>
          </div>
        </div>
      </div>
    </section>
  )
}
