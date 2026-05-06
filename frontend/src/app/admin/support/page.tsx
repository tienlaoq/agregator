"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { formatApiErrorMessage, listAdminSupportTickets, submitSupportTicketReply } from "@/lib/api";
import type { SupportTicketAdmin } from "@/lib/types";
import { useAuthStore } from "@/store/auth";
import { ChevronLeft, ChevronRight, Mail } from "lucide-react";

const PAGE_SIZE = 25;

function NotifyDeliveryBadge({ status }: { status?: string }) {
  if (status === "failed") {
    return (
      <Badge variant="destructive" title="Не удалось отправить email модераторам или webhook">
        Ошибка
      </Badge>
    );
  }
  if (status === "pending") {
    return <Badge variant="outline">В процессе</Badge>;
  }
  if (status === "ok") {
    return <Badge variant="secondary">Доставлено</Badge>;
  }
  return <span className="text-muted-foreground text-xs">—</span>;
}

export default function AdminSupportPage() {
  const router = useRouter();
  const { token, user, hydrated } = useAuthStore();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(0);
  const [active, setActive] = useState<SupportTicketAdmin | null>(null);
  const [replyText, setReplyText] = useState("");
  const [replyError, setReplyError] = useState("");

  useEffect(() => {
    if (hydrated && (!token || user?.role !== "admin")) {
      router.push("/");
    }
  }, [hydrated, token, user, router]);

  const listQuery = useQuery({
    queryKey: ["admin-support-tickets", page],
    queryFn: () => listAdminSupportTickets({ limit: PAGE_SIZE, offset: page * PAGE_SIZE }),
    enabled: hydrated && !!token && user?.role === "admin",
  });

  const replyMutation = useMutation({
    mutationFn: async () => {
      if (!active) throw new Error("no ticket");
      return submitSupportTicketReply({
        request_id: active.request_id,
        ticket_number: active.ticket_number,
        user_email: active.user_email,
        reply: replyText.trim(),
      });
    },
    onSuccess: async () => {
      setReplyError("");
      setReplyText("");
      setActive(null);
      await queryClient.invalidateQueries({ queryKey: ["admin-support-tickets"] });
    },
    onError: (e: unknown) => {
      setReplyError(formatApiErrorMessage(e, "Не удалось отправить ответ."));
    },
  });

  const total = listQuery.data?.total ?? 0;
  const tickets = listQuery.data?.tickets ?? [];
  const lastPage = Math.max(0, Math.ceil(total / PAGE_SIZE) - 1);

  const openReply = (t: SupportTicketAdmin) => {
    setActive(t);
    setReplyText("");
    setReplyError("");
  };

  if (!hydrated || !token || user?.role !== "admin") {
    return (
      <section className="bg-background py-10 md:py-16">
        <div className="container mx-auto max-w-5xl px-4 text-sm text-muted-foreground">Загрузка…</div>
      </section>
    );
  }

  return (
    <section className="bg-background py-10 md:py-16">
      <div className="container mx-auto max-w-6xl px-4 space-y-6">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <Button variant="ghost" size="sm" asChild className="-ml-2 mb-1">
              <Link href="/admin/venues">← Назад в админку</Link>
            </Button>
            <h1 className="text-2xl font-semibold tracking-tight">Обращения в поддержку</h1>
            <p className="text-sm text-muted-foreground mt-1">
              Все заявки сохраняются в базе. Ответ уходит пользователю на email через SMTP шлюза.
            </p>
          </div>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>Очередь</CardTitle>
            <CardDescription>
              Всего записей: {listQuery.isFetching ? "…" : total}. Нажмите «Ответить», чтобы отправить письмо на адрес из
              обращения.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {listQuery.isError ? (
              <p className="text-sm text-destructive">
                {formatApiErrorMessage(listQuery.error, "Не удалось загрузить обращения. Проверьте миграции support_db и PG_* у api-gateway.")}
              </p>
            ) : null}
            <div className="rounded-md border overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="whitespace-nowrap">Создано</TableHead>
                    <TableHead>Номер</TableHead>
                    <TableHead>Тема</TableHead>
                    <TableHead>Email</TableHead>
                    <TableHead className="whitespace-nowrap">Уведомл.</TableHead>
                    <TableHead>Статус</TableHead>
                    <TableHead className="text-right">Действие</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {listQuery.isLoading ? (
                    <TableRow>
                      <TableCell colSpan={7} className="text-muted-foreground">
                        Загрузка…
                      </TableCell>
                    </TableRow>
                  ) : tickets.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={7} className="text-muted-foreground">
                        Обращений пока нет.
                      </TableCell>
                    </TableRow>
                  ) : (
                    tickets.map((t) => (
                      <TableRow key={t.request_id}>
                        <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                          {new Date(t.created_at).toLocaleString("ru-RU")}
                        </TableCell>
                        <TableCell className="font-mono text-xs">{t.ticket_number}</TableCell>
                        <TableCell className="max-w-[200px] truncate" title={t.topic}>
                          {t.topic}
                        </TableCell>
                        <TableCell className="max-w-[180px] truncate text-sm">{t.user_email || "—"}</TableCell>
                        <TableCell>
                          <NotifyDeliveryBadge status={t.notify_status} />
                        </TableCell>
                        <TableCell>
                          {t.replied_at ? (
                            <Badge variant="secondary">Отвечено</Badge>
                          ) : (
                            <Badge variant="outline">Новое</Badge>
                          )}
                        </TableCell>
                        <TableCell className="text-right">
                          <Button type="button" size="sm" variant="outline" onClick={() => openReply(t)}>
                            <Mail className="h-3.5 w-3.5 mr-1" />
                            Ответить
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
            <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
              <span className="text-muted-foreground">
                Страница {page + 1} из {total === 0 ? 1 : lastPage + 1}
              </span>
              <div className="flex gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={page <= 0 || listQuery.isFetching}
                  onClick={() => setPage((p) => Math.max(0, p - 1))}
                >
                  <ChevronLeft className="h-4 w-4" />
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={page >= lastPage || listQuery.isFetching || total === 0}
                  onClick={() => setPage((p) => p + 1)}
                >
                  <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>

        <Dialog
          open={active !== null}
          onOpenChange={(open) => {
            if (!open) {
              setActive(null);
              setReplyError("");
            }
          }}
        >
          <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
            <DialogHeader>
              <DialogTitle>Ответ пользователю</DialogTitle>
              <DialogDescription>
                Письмо уйдёт на <span className="font-medium text-foreground">{active?.user_email}</span> ·{" "}
                <span className="font-mono text-xs">{active?.ticket_number}</span>
              </DialogDescription>
            </DialogHeader>
            {active ? (
              <div className="space-y-3 text-sm">
                <div>
                  <p className="text-muted-foreground text-xs uppercase tracking-wide">Тема</p>
                  <p className="font-medium">{active.topic}</p>
                </div>
                <div>
                  <p className="text-muted-foreground text-xs uppercase tracking-wide">Текст обращения</p>
                  <p className="whitespace-pre-wrap rounded-md border bg-muted/40 p-3 text-sm">{active.message}</p>
                </div>
                {(active.booking_id || active.payment_id) && (
                  <p className="text-xs text-muted-foreground">
                    {active.booking_id ? `booking_id: ${active.booking_id}` : ""}{" "}
                    {active.payment_id ? `payment_id: ${active.payment_id}` : ""}
                  </p>
                )}
                <div className="space-y-2 pt-2">
                  <Label htmlFor="admin-support-reply">Ответ</Label>
                  <Textarea
                    id="admin-support-reply"
                    value={replyText}
                    onChange={(e) => setReplyText(e.target.value)}
                    placeholder="Текст ответа в письме пользователю…"
                    className="min-h-[140px]"
                  />
                </div>
                {replyError ? <p className="text-sm text-destructive">{replyError}</p> : null}
              </div>
            ) : null}
            <DialogFooter className="gap-2 sm:gap-0">
              <Button type="button" variant="outline" onClick={() => setActive(null)}>
                Отмена
              </Button>
              <Button
                type="button"
                disabled={replyMutation.isPending || !replyText.trim() || !active?.user_email}
                onClick={() => replyMutation.mutate()}
              >
                {replyMutation.isPending ? "Отправка…" : "Отправить на почту"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </section>
  );
}
