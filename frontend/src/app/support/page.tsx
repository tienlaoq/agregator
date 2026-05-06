"use client";

import { FormEvent, useEffect, useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useMutation } from "@tanstack/react-query";
import Link from "next/link";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { formatApiErrorMessage, submitSupportContact } from "@/lib/api";
import { useAuthStore } from "@/store/auth";

function SupportUserForm() {
  const searchParams = useSearchParams();
  const user = useAuthStore((s) => s.user);
  const [topic, setTopic] = useState("");
  const [message, setMessage] = useState("");
  const [email, setEmail] = useState(user?.email ?? "");
  const [requestID, setRequestID] = useState("");
  const [ticketNumber, setTicketNumber] = useState("");
  const [error, setError] = useState("");

  const bookingID = useMemo(() => (searchParams.get("booking_id") || "").trim(), [searchParams]);
  const paymentID = useMemo(() => (searchParams.get("payment_id") || "").trim(), [searchParams]);
  const sourcePage = useMemo(() => (searchParams.get("source") || "").trim(), [searchParams]);

  const submitMutation = useMutation({
    mutationFn: () =>
      submitSupportContact({
        topic: topic.trim(),
        message: message.trim(),
        email: email.trim(),
        booking_id: bookingID || undefined,
        payment_id: paymentID || undefined,
        source_page:
          sourcePage || (typeof window !== "undefined" ? window.location.pathname + window.location.search : ""),
      }),
    onSuccess: (resp) => {
      setRequestID(resp.request_id);
      setTicketNumber((resp.ticket_number || "").trim() || `SUP-${resp.request_id.slice(0, 8).toUpperCase()}`);
      setError("");
      setMessage("");
    },
    onError: (e: unknown) => {
      setError(formatApiErrorMessage(e, "Не удалось отправить обращение. Попробуйте позже."));
    },
  });

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    setRequestID("");
    setTicketNumber("");
    setError("");
    submitMutation.mutate();
  };

  return (
    <section className="bg-background py-10 md:py-16">
      <div className="container mx-auto max-w-2xl px-4">
        <Card>
          <CardHeader>
            <CardTitle>Поддержка</CardTitle>
            <CardDescription>
              Опишите проблему, и команда поддержки ответит вам в рабочее время.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-4" onSubmit={onSubmit}>
              <div className="space-y-2">
                <Label htmlFor="support-topic">Тема</Label>
                <Input
                  id="support-topic"
                  value={topic}
                  onChange={(e) => setTopic(e.target.value)}
                  placeholder="Например: проблема с оплатой"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="support-message">Описание</Label>
                <Textarea
                  id="support-message"
                  value={message}
                  onChange={(e) => setMessage(e.target.value)}
                  placeholder="Что произошло и когда?"
                  className="min-h-[140px]"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="support-email">Email для ответа</Label>
                <Input
                  id="support-email"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="name@example.com"
                />
              </div>

              {bookingID || paymentID ? (
                <p className="text-xs text-muted-foreground">
                  Контекст: {bookingID ? `booking_id=${bookingID}` : ""}{" "}
                  {paymentID ? `payment_id=${paymentID}` : ""}
                </p>
              ) : null}

              {error ? <p className="text-sm text-destructive">{error}</p> : null}
              {requestID ? (
                <p className="text-sm text-emerald-600">
                  Обращение отправлено. Номер обращения: <span className="font-mono">{ticketNumber}</span>
                </p>
              ) : null}

              <div className="flex flex-wrap gap-2">
                <Button type="submit" disabled={submitMutation.isPending || !topic.trim() || !message.trim()}>
                  {submitMutation.isPending ? "Отправляем..." : "Отправить обращение"}
                </Button>
                <Button asChild variant="outline">
                  <Link href="/my/bookings">Назад к бронированиям</Link>
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      </div>
    </section>
  );
}

function AdminSupportRedirect() {
  const router = useRouter();
  useEffect(() => {
    router.replace("/admin/support");
  }, [router]);
  return (
    <section className="bg-background py-10 md:py-16">
      <div className="container mx-auto max-w-2xl px-4 text-sm text-muted-foreground">Переход в раздел обращений…</div>
    </section>
  );
}

export default function SupportPage() {
  const hydrated = useAuthStore((s) => s.hydrated);
  const user = useAuthStore((s) => s.user);

  if (!hydrated) {
    return (
      <section className="bg-background py-10 md:py-16">
        <div className="container mx-auto max-w-2xl px-4 text-sm text-muted-foreground">Загрузка…</div>
      </section>
    );
  }

  if (user?.role === "admin") {
    return <AdminSupportRedirect />;
  }

  return <SupportUserForm />;
}
