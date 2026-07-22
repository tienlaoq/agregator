"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { createBooking, formatApiErrorMessage } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import { Calendar, Clock, Users } from "lucide-react";

interface BookingFormProps {
  venueId: string;
  venueName: string;
  priceFrom: number;
}

const TIME_SLOTS = [
  "09:00", "10:00", "11:00", "12:00", "13:00", "14:00",
  "15:00", "16:00", "17:00", "18:00", "19:00", "20:00", "21:00",
];

function localDateMinYYYYMMDD(): string {
  const d = new Date();
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export function BookingForm({ venueId, venueName, priceFrom }: BookingFormProps) {
  const { token } = useAuthStore();
  const router = useRouter();
  const [date, setDate] = useState("");
  const [time, setTime] = useState("");
  const [guests, setGuests] = useState(2);
  const [accepted, setAccepted] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) {
      router.push("/auth/login");
      return;
    }
    setError("");
    const todayLocal = localDateMinYYYYMMDD();
    if (!date || date < todayLocal) {
      setError("Выберите сегодняшнюю или будущую дату.");
      return;
    }
    setLoading(true);
    try {
      await createBooking({ venue_id: venueId, date, time_from: time, guests });
      setSuccess(true);
    } catch (e) {
      setError(
        formatApiErrorMessage(
          e,
          "Не удалось создать бронирование. Попробуйте позже.",
        ),
      );
    } finally {
      setLoading(false);
    }
  };

  if (success) {
    return (
      <Card className="border-green-200 bg-green-50">
        <CardContent className="pt-4 text-center">
          <p className="mb-2 text-lg font-semibold text-green-800">
            Бронирование создано!
          </p>
          <p className="text-sm text-green-600">
            {venueName} &middot; {date} в {time} &middot; {guests} чел.
          </p>
          <Button
            variant="outline"
            size="sm"
            className="mt-4"
            onClick={() => router.push("/my/bookings")}
          >
            Мои бронирования
          </Button>
        </CardContent>
      </Card>
    );
  }

  const today = localDateMinYYYYMMDD();

  return (
    <Card>
      <CardHeader>
        <CardTitle>Забронировать</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="date">
              <Calendar className="h-4 w-4" />
              Дата
            </Label>
            <Input
              id="date"
              type="date"
              min={today}
              value={date}
              onChange={(e) => setDate(e.target.value)}
              required
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="time">
              <Clock className="h-4 w-4" />
              Время
            </Label>
            <select
              id="time"
              value={time}
              onChange={(e) => setTime(e.target.value)}
              required
              className="flex h-8 w-full rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
            >
              <option value="">Выберите время</option>
              {TIME_SLOTS.map((slot) => (
                <option key={slot} value={slot}>
                  {slot}
                </option>
              ))}
            </select>
          </div>

          <div className="space-y-2">
            <Label htmlFor="guests">
              <Users className="h-4 w-4" />
              Гости
            </Label>
            <Input
              id="guests"
              type="number"
              min={1}
              max={20}
              value={guests}
              onChange={(e) => setGuests(Number(e.target.value))}
              required
            />
          </div>

          <div className="rounded-md bg-muted/50 p-3">
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">Стоимость от</span>
              <span className="font-semibold">
                {(priceFrom * guests).toLocaleString("ru-RU")} ₽
              </span>
            </div>
          </div>

          {token && (
            <div className="flex items-start gap-2.5">
              <Checkbox
                id="offer-accept"
                checked={accepted}
                onCheckedChange={(v) => setAccepted(v === true)}
                className="mt-0.5"
                aria-required
              />
              <label htmlFor="offer-accept" className="text-sm leading-snug text-muted-foreground">
                Я принимаю условия{" "}
                <Link href="/offer" target="_blank" className="text-primary underline">
                  Публичной оферты
                </Link>{" "}
                и соглашаюсь с Правилами посещения бань.
              </label>
            </div>
          )}

          {error && (
            <p className="text-sm text-destructive">{error}</p>
          )}

          <Button type="submit" className="w-full" disabled={loading || (!!token && !accepted)}>
            {loading ? "Оформление..." : token ? "Забронировать" : "Войти для бронирования"}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
