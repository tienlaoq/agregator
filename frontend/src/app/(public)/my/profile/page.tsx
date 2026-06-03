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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { getProfile, updateProfile, deleteAccount, ApiError } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import { PhoneInput } from "@/components/banya/phone-input";

export default function ProfilePage() {
  const router = useRouter();
  const { token, hydrated, setUser, logout } = useAuthStore();
  const [saved, setSaved] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [confirmation, setConfirmation] = useState("");

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

  const deleteMutation = useMutation({
    mutationFn: (conf: string) => deleteAccount(conf),
    onSuccess: () => {
      logout();
      router.push("/");
    },
  });

  const isAdmin = profile?.role === "admin";

  const deleteErrorMessage = (() => {
    const err = deleteMutation.error;
    if (!err) return null;
    if (err instanceof ApiError) {
      switch (err.code) {
        case "GATEWAY.ACCOUNT.CONFIRMATION_MISMATCH":
          return "Подтверждение не совпадает с email вашего аккаунта";
        case "GATEWAY.ACCOUNT.CONFIRMATION_REQUIRED":
          return "Введите email для подтверждения";
        case "GATEWAY.ACCOUNT.ADMIN_SELF_DELETE_FORBIDDEN":
          return "Аккаунт администратора нельзя удалить через профиль";
        default:
          return err.gatewayDetail || "Не удалось удалить аккаунт";
      }
    }
    return "Не удалось удалить аккаунт";
  })();

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
              variant="outline"
              className="w-full"
              onClick={handleLogout}
            >
              Выйти из аккаунта
            </Button>

            {!isAdmin && (
              <>
                <Separator className="my-6" />
                <div className="space-y-2">
                  <h2 className="text-sm font-semibold text-destructive">
                    Удаление аккаунта
                  </h2>
                  <p className="text-sm text-muted-foreground">
                    Аккаунт будет деактивирован без возможности восстановления.
                    {profile?.role === "venue_owner" &&
                      " Ваши заведения будут сняты с публикации."}
                    {profile?.role === "master" &&
                      " Ваш профиль мастера будет скрыт."}
                  </p>
                  <Button
                    variant="destructive"
                    className="w-full"
                    onClick={() => {
                      setConfirmation("");
                      deleteMutation.reset();
                      setDeleteOpen(true);
                    }}
                  >
                    Удалить аккаунт
                  </Button>
                </div>
              </>
            )}
          </CardContent>
        </Card>
      )}

      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Удалить аккаунт?</DialogTitle>
            <DialogDescription>
              Это действие необратимо. Для подтверждения введите email вашего
              аккаунта:{" "}
              <span className="font-medium text-foreground">
                {profile?.email}
              </span>
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-2">
            <Label htmlFor="delete-confirmation">Email для подтверждения</Label>
            <Input
              id="delete-confirmation"
              type="email"
              autoComplete="off"
              value={confirmation}
              onChange={(e) => setConfirmation(e.target.value)}
              placeholder={profile?.email ?? ""}
            />
            {deleteErrorMessage && (
              <p className="text-sm text-destructive">{deleteErrorMessage}</p>
            )}
          </div>

          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setDeleteOpen(false)}
              disabled={deleteMutation.isPending}
            >
              Отмена
            </Button>
            <Button
              variant="destructive"
              onClick={() => deleteMutation.mutate(confirmation)}
              disabled={
                deleteMutation.isPending ||
                confirmation.trim().toLowerCase() !==
                  (profile?.email ?? "").trim().toLowerCase()
              }
            >
              {deleteMutation.isPending ? "Удаление..." : "Удалить навсегда"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
