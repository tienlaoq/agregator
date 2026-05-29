"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery, useMutation } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { getProfile, updateProfile } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import { PhoneInput } from "@/components/banya/phone-input";

export default function ProfilePage() {
  const router = useRouter();
  const { token, hydrated, setUser, logout } = useAuthStore();
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    if (hydrated && !token) router.push("/auth/login");
  }, [hydrated, token, router]);

  const { data: profile, isLoading } = useQuery({
    queryKey: ["profile"],
    queryFn: getProfile,
    enabled: !!token,
  });

  const initialName = profile?.name ?? "";
  const initialPhone = profile?.phone ?? "";
  const [name, setName] = useState("");
  const [phone, setPhone] = useState("");
  const nameValue = name || initialName;
  const phoneValue = phone || initialPhone;

  const mutation = useMutation({
    mutationFn: (data: { name: string; phone: string }) => updateProfile(data),
    onSuccess: (user) => {
      setUser(user);
      setSaved(true);
      setTimeout(() => setSaved(false), 3000);
    },
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    mutation.mutate({ name: nameValue, phone: phoneValue });
  };

  const handleLogout = () => {
    logout();
    router.push("/");
  };

  if (!hydrated || !token) return null;

  return (
    <div className="mx-auto max-w-lg px-4 py-8">
      <h1 className="mb-6 text-2xl font-bold">Мой профиль</h1>

      {isLoading ? (
        <div className="space-y-4">
          <div className="h-12 animate-pulse rounded bg-muted" />
          <div className="h-12 animate-pulse rounded bg-muted" />
        </div>
      ) : (
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle>Личные данные</CardTitle>
              {profile && (
                <Badge variant="secondary">
                  {profile.role === "venue_owner"
                    ? "Владелец"
                    : profile.role === "master"
                      ? "Пар-мастер"
                      : profile.role === "admin"
                        ? "Администратор"
                        : "Посетитель"}
                </Badge>
              )}
            </div>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="email">Email</Label>
                <Input
                  id="email"
                  value={profile?.email ?? ""}
                  disabled
                  className="bg-muted/50"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="name">Имя</Label>
                <Input
                  id="name"
                  value={nameValue}
                  onChange={(e) => setName(e.target.value)}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="phone">Телефон</Label>
                <PhoneInput
                  id="phone"
                  value={phoneValue}
                  onChange={setPhone}
                  required
                />
              </div>

              {mutation.isError && (
                <p className="text-sm text-destructive">
                  Не удалось сохранить изменения
                </p>
              )}
              {saved && (
                <p className="text-sm text-green-600">
                  Изменения сохранены
                </p>
              )}

              <Button
                type="submit"
                className="w-full"
                disabled={mutation.isPending}
              >
                {mutation.isPending ? "Сохранение..." : "Сохранить"}
              </Button>
            </form>

            <Separator className="my-6" />

            <Button
              variant="destructive"
              className="w-full"
              onClick={handleLogout}
            >
              Выйти из аккаунта
            </Button>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
