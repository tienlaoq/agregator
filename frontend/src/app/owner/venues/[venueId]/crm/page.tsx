"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import {
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Badge } from "@/components/ui/badge";
import { redirectToLogin } from "@/lib/auth-redirect";
import {
  addBookingStaffNote,
  completeVenueCrmTask,
  createVenueCrmTask,
  formatApiErrorMessage,
  getOwnerVenueBookings,
  getOwnerVenues,
  inviteVenueStaffByEmail,
  listBookingStaffNotes,
  listVenueCrmTasks,
  listVenueStaff,
  removeVenueStaff,
} from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import type { Booking, Venue, VenueCrmTask, VenueStaffRow } from "@/lib/types";
import { BOOKING_STATUS_LABELS, VENUE_TYPE_LABELS } from "@/lib/types";
import {
  ArrowLeft,
  CheckCircle2,
  ChevronDown,
  Loader2,
  Pencil,
  Trash2,
  UserPlus,
} from "lucide-react";
import { BookingChatPanel } from "@/components/banya/booking-chat-panel";

const uuidRe =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function canManageVenueStaff(venue: Venue, userId: string | undefined): boolean {
  if (!userId) return false;
  if (venue.management_access === "owner") return true;
  if (!venue.management_access && venue.owner_id === userId) return true;
  return false;
}

function formatDt(iso: string | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("ru-RU", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

const VENUE_CRM_ROLE_LABELS: Record<string, string> = {
  manager: "Менеджер",
  staff: "Сотрудник",
};

function venueCrmRoleLabel(role: string): string {
  return VENUE_CRM_ROLE_LABELS[role] ?? role;
}

/** Имя из профиля пользователя; иначе email; иначе подсказка (не нейтральная «команда»). */
function venueStaffMemberPrimary(s: VenueStaffRow): string {
  const n = s.user_name?.trim();
  if (n) return n;
  const e = s.user_email?.trim();
  if (e) return e;
  return "Имя не указано в профиле";
}

function venueStaffMemberSubtitle(s: VenueStaffRow): string | null {
  const n = s.user_name?.trim();
  const e = s.user_email?.trim();
  if (n && e) return e;
  return null;
}

function venueStaffInviterLabel(
  s: VenueStaffRow,
  currentUserId: string | undefined,
): string {
  if (s.inviter_is_you || (currentUserId && s.invited_by === currentUserId)) {
    return "Вы";
  }
  const n = s.inviter_name?.trim();
  if (n) return n;
  const e = s.inviter_email?.trim();
  if (e) return e;
  return "—";
}

export default function OwnerVenueCrmPage() {
  const params = useParams<{ venueId: string }>();
  const venueId = params.venueId;
  const queryClient = useQueryClient();
  const { token, user, hydrated } = useAuthStore();

  const [taskFilter, setTaskFilter] = useState<string>("");
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteRole, setInviteRole] = useState("staff");
  const [taskTitle, setTaskTitle] = useState("");
  const [taskBody, setTaskBody] = useState("");
  const [taskBookingId, setTaskBookingId] = useState("");
  const [pageError, setPageError] = useState("");
  const [inviteError, setInviteError] = useState("");
  const [taskError, setTaskError] = useState("");
  const [expandedBookingId, setExpandedBookingId] = useState<string | null>(
    null,
  );
  const [noteDrafts, setNoteDrafts] = useState<Record<string, string>>({});

  const validId = typeof venueId === "string" && uuidRe.test(venueId);
  const canOwnerCabinet =
    user?.role === "venue_owner" ||
    user?.role === "master" ||
    user?.role === "user";
  const canEditVenueCard =
    user?.role === "venue_owner" || user?.role === "master";

  useEffect(() => {
    if (hydrated && (!token || !canOwnerCabinet)) {
      redirectToLogin();
    }
  }, [hydrated, token, canOwnerCabinet]);

  const { data: venues, isLoading: venuesLoading } = useQuery({
    queryKey: ["owner-venues"],
    queryFn: getOwnerVenues,
    enabled: !!token && canOwnerCabinet && validId,
  });

  const venue = useMemo(
    () => venues?.find((v) => v.id === venueId) ?? null,
    [venues, venueId],
  );

  const { data: staff = [], isLoading: staffLoading } = useQuery({
    queryKey: ["venue-crm-staff", venueId],
    queryFn: () => listVenueStaff(venueId as string),
    enabled: !!token && validId && !!venue,
  });

  const { data: tasks = [], isLoading: tasksLoading } = useQuery({
    queryKey: ["venue-crm-tasks", venueId, taskFilter],
    queryFn: () =>
      listVenueCrmTasks(venueId as string, {
        status: taskFilter || undefined,
      }),
    enabled: !!token && validId && !!venue,
  });

  const { data: bookingsData, isLoading: bookingsLoading } = useQuery({
    queryKey: ["owner-venue-bookings", venueId, "crm"],
    queryFn: () => getOwnerVenueBookings(venueId as string, { page_size: 80 }),
    enabled: !!token && validId && !!venue,
  });

  const bookings = bookingsData?.bookings ?? [];

  const inviteMu = useMutation({
    mutationFn: () =>
      inviteVenueStaffByEmail(venueId as string, {
        email: inviteEmail.trim(),
        role: inviteRole,
      }),
    onSuccess: () => {
      setInviteError("");
      setInviteEmail("");
      void queryClient.invalidateQueries({ queryKey: ["venue-crm-staff", venueId] });
    },
    onError: (e: unknown) => {
      setInviteError(formatApiErrorMessage(e, "Не удалось пригласить."));
    },
  });

  const removeStaffMu = useMutation({
    mutationFn: (userId: string) => removeVenueStaff(venueId as string, userId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["venue-crm-staff", venueId] });
    },
    onError: (e: unknown) => {
      setPageError(formatApiErrorMessage(e, "Не удалось удалить из команды."));
    },
  });

  const createTaskMu = useMutation({
    mutationFn: () =>
      createVenueCrmTask(venueId as string, {
        title: taskTitle.trim(),
        body: taskBody.trim(),
        booking_id: taskBookingId.trim() || undefined,
      }),
    onSuccess: () => {
      setTaskError("");
      setTaskTitle("");
      setTaskBody("");
      setTaskBookingId("");
      void queryClient.invalidateQueries({ queryKey: ["venue-crm-tasks", venueId] });
    },
    onError: (e: unknown) => {
      setTaskError(formatApiErrorMessage(e, "Не удалось создать задачу."));
    },
  });

  const completeTaskMu = useMutation({
    mutationFn: (taskId: string) =>
      completeVenueCrmTask(venueId as string, taskId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["venue-crm-tasks", venueId] });
    },
    onError: (e: unknown) => {
      setPageError(formatApiErrorMessage(e, "Не удалось закрыть задачу."));
    },
  });

  const notesQueries = useQuery({
    queryKey: ["booking-staff-notes", venueId, expandedBookingId],
    queryFn: () =>
      listBookingStaffNotes(venueId as string, expandedBookingId as string),
    enabled: !!expandedBookingId && !!token && validId,
  });

  const addNoteMu = useMutation({
    mutationFn: ({ bookingId, body }: { bookingId: string; body: string }) =>
      addBookingStaffNote(venueId as string, bookingId, body),
    onSuccess: (_, v) => {
      setNoteDrafts((d) => ({ ...d, [v.bookingId]: "" }));
      void queryClient.invalidateQueries({
        queryKey: ["booking-staff-notes", venueId, v.bookingId],
      });
    },
    onError: (e: unknown) => {
      setPageError(formatApiErrorMessage(e, "Не удалось сохранить заметку."));
    },
  });

  useEffect(() => {
    if (!venuesLoading && validId && venues && !venue) {
      setPageError("Заведение не найдено в вашем доступе.");
    }
  }, [venuesLoading, validId, venues, venue]);

  if (!hydrated || !token) return null;

  if (!validId) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-10">
        <p className="text-sm text-muted-foreground">Некорректная ссылка.</p>
        <Button asChild variant="link" className="mt-2 h-auto px-0 text-sm">
          <Link href="/owner/venues">← Панель</Link>
        </Button>
      </div>
    );
  }

  if (venuesLoading || !venues) {
    return (
      <div className="min-h-screen bg-muted/30 py-10">
        <div className="container mx-auto px-4">
          <div className="mx-auto max-w-5xl">
            <div className="h-40 animate-pulse rounded-lg border border-border bg-muted" />
          </div>
        </div>
      </div>
    );
  }

  if (!venue) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-10">
        <p className="text-sm text-destructive">{pageError || "Заведение не найдено."}</p>
        <Button asChild variant="link" className="mt-2 h-auto px-0 text-sm">
          <Link href="/owner/venues">← Панель</Link>
        </Button>
      </div>
    );
  }

  const staffManage = canManageVenueStaff(venue, user?.id);

  return (
    <section className="min-h-screen bg-muted/30 py-8 md:py-10">
      <div className="container mx-auto max-w-5xl px-4">
        <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex flex-wrap items-center gap-3">
            <Button variant="ghost" size="sm" className="gap-1 px-0" asChild>
              <Link href="/owner/venues">
                <ArrowLeft className="h-4 w-4" />
                Панель
              </Link>
            </Button>
            <h1 className="text-2xl font-bold text-foreground md:text-3xl">
              CRM: {venue.name}
            </h1>
            <Badge variant="secondary">
              {VENUE_TYPE_LABELS[venue.type] ?? venue.type}
            </Badge>
            {venue.management_access ? (
              <Badge variant="outline">{venue.management_access}</Badge>
            ) : null}
          </div>
          {canEditVenueCard ? (
            <Button variant="outline" size="sm" className="gap-1" asChild>
              <Link href={`/owner/venues/${venue.id}/edit`}>
                <Pencil className="h-4 w-4" />
                Карточка заведения
              </Link>
            </Button>
          ) : null}
        </div>

        {pageError ? (
          <p className="mb-4 text-sm text-destructive" role="alert">
            {pageError}
          </p>
        ) : null}

        <div className="grid gap-6 lg:grid-cols-2">
          <Card className="border-border">
            <CardHeader>
              <CardTitle>Задачи</CardTitle>
              <CardDescription>
                Внутренние задачи по заведению (не видны гостям).
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex flex-wrap items-end gap-3">
                <div className="space-y-2">
                  <Label>Статус</Label>
                  <Select
                    value={taskFilter || "all"}
                    onValueChange={(v) => setTaskFilter(v === "all" ? "" : v)}
                  >
                    <SelectTrigger className="w-[180px]">
                      <SelectValue placeholder="Все" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">Все</SelectItem>
                      <SelectItem value="open">Открытые</SelectItem>
                      <SelectItem value="done">Закрытые</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>

              <div className="rounded-lg border border-border">
                {tasksLoading ? (
                  <div className="flex items-center justify-center gap-2 p-8 text-muted-foreground">
                    <Loader2 className="h-5 w-5 animate-spin" />
                    Загрузка…
                  </div>
                ) : tasks.length === 0 ? (
                  <p className="p-6 text-sm text-muted-foreground">Задач пока нет.</p>
                ) : (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Задача</TableHead>
                        <TableHead>Статус</TableHead>
                        <TableHead className="text-right">Действия</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {tasks.map((t: VenueCrmTask) => (
                        <TableRow key={t.id}>
                          <TableCell>
                            <div className="font-medium">{t.title}</div>
                            {t.body ? (
                              <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                                {t.body}
                              </p>
                            ) : null}
                          </TableCell>
                          <TableCell>
                            <Badge
                              variant={
                                t.status === "done" ? "secondary" : "default"
                              }
                            >
                              {t.status === "done" ? "Готово" : "Открыта"}
                            </Badge>
                          </TableCell>
                          <TableCell className="text-right">
                            {t.status === "open" ? (
                              <Button
                                type="button"
                                size="sm"
                                variant="outline"
                                disabled={completeTaskMu.isPending}
                                onClick={() => completeTaskMu.mutate(t.id)}
                              >
                                <CheckCircle2 className="mr-1 h-3.5 w-3.5" />
                                Закрыть
                              </Button>
                            ) : null}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                )}
              </div>

              <div className="space-y-3 border-t border-border pt-4">
                <Label>Новая задача</Label>
                {taskError ? (
                  <p className="text-sm text-destructive">{taskError}</p>
                ) : null}
                <Input
                  placeholder="Заголовок"
                  value={taskTitle}
                  onChange={(e) => setTaskTitle(e.target.value)}
                />
                <Textarea
                  placeholder="Описание (необязательно)"
                  value={taskBody}
                  onChange={(e) => setTaskBody(e.target.value)}
                  className="min-h-[80px]"
                />
                <div className="space-y-2">
                  <Label htmlFor="task-booking-id" className="text-foreground">
                    Задача про конкретное бронирование?
                  </Label>
                  <p className="text-xs text-muted-foreground">
                    Оставьте пустым — задача общая по заведению. Если нужно привязать к одному визиту,
                    вставьте технический номер брони (UUID), например из письма поддержки или
                    внутренних инструментов — в списке броней ниже этот номер не показывается.
                  </p>
                  <Input
                    id="task-booking-id"
                    placeholder="Технический номер брони — только если известен"
                    value={taskBookingId}
                    onChange={(e) => setTaskBookingId(e.target.value)}
                    autoComplete="off"
                  />
                </div>
                <Button
                  type="button"
                  disabled={
                    createTaskMu.isPending || !taskTitle.trim()
                  }
                  onClick={() => createTaskMu.mutate()}
                >
                  {createTaskMu.isPending ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Создание…
                    </>
                  ) : (
                    "Создать задачу"
                  )}
                </Button>
              </div>
            </CardContent>
          </Card>

          <Card className="border-border">
            <CardHeader>
              <CardTitle>Команда</CardTitle>
              <CardDescription>
                {staffManage
                  ? "Приглашайте по email зарегистрированных пользователей. Роли: сотрудник (операции по заведению), менеджер (расширенные права)."
                  : "Список сотрудников с доступом к операциям по заведению."}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {staffManage ? (
                <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
                  <div className="min-w-0 flex-1 space-y-2">
                    <Label htmlFor="invite-email">Email</Label>
                    <Input
                      id="invite-email"
                      type="email"
                      placeholder="colleague@example.com"
                      value={inviteEmail}
                      onChange={(e) => setInviteEmail(e.target.value)}
                    />
                  </div>
                  <div className="w-full space-y-2 sm:w-40">
                    <Label>Роль</Label>
                    <Select value={inviteRole} onValueChange={setInviteRole}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="staff">Сотрудник</SelectItem>
                        <SelectItem value="manager">Менеджер</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <Button
                    type="button"
                    className="gap-1 sm:mb-0"
                    disabled={inviteMu.isPending || !inviteEmail.trim()}
                    onClick={() => inviteMu.mutate()}
                  >
                    {inviteMu.isPending ? (
                      <Loader2 className="h-4 w-4 animate-spin" />
                    ) : (
                      <UserPlus className="h-4 w-4" />
                    )}
                    Пригласить
                  </Button>
                </div>
              ) : null}
              {inviteError ? (
                <p className="text-sm text-destructive">{inviteError}</p>
              ) : null}

              <div className="rounded-lg border border-border">
                {staffLoading ? (
                  <div className="flex items-center justify-center gap-2 p-8 text-muted-foreground">
                    <Loader2 className="h-5 w-5 animate-spin" />
                    Загрузка…
                  </div>
                ) : staff.length === 0 ? (
                  <p className="p-6 text-sm text-muted-foreground">
                    Пока только владелец (записей персонала нет).
                  </p>
                ) : (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Сотрудник</TableHead>
                        <TableHead>Роль</TableHead>
                        <TableHead>Пригласил</TableHead>
                        <TableHead>Дата приглашения</TableHead>
                        {staffManage ? (
                          <TableHead className="text-right">Действия</TableHead>
                        ) : null}
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {staff.map((s: VenueStaffRow) => (
                        <TableRow key={s.user_id}>
                          <TableCell>
                            <div className="font-medium text-foreground">
                              {venueStaffMemberPrimary(s)}
                            </div>
                            {venueStaffMemberSubtitle(s) ? (
                              <div className="mt-0.5 text-xs text-muted-foreground">
                                {venueStaffMemberSubtitle(s)}
                              </div>
                            ) : null}
                          </TableCell>
                          <TableCell>
                            <Badge variant="secondary" className="font-normal">
                              {venueCrmRoleLabel(s.role)}
                            </Badge>
                          </TableCell>
                          <TableCell className="text-sm text-foreground">
                            {venueStaffInviterLabel(s, user?.id)}
                          </TableCell>
                          <TableCell className="text-sm text-muted-foreground">
                            {formatDt(s.created_at)}
                          </TableCell>
                          {staffManage ? (
                            <TableCell className="text-right">
                              <Button
                                type="button"
                                size="sm"
                                variant="ghost"
                                className="text-destructive hover:text-destructive"
                                disabled={removeStaffMu.isPending}
                                onClick={() => removeStaffMu.mutate(s.user_id)}
                                title="Убрать из команды"
                              >
                                <Trash2 className="h-4 w-4" />
                              </Button>
                            </TableCell>
                          ) : null}
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                )}
              </div>
            </CardContent>
          </Card>
        </div>

        <Card className="mt-6 border-border">
          <CardHeader>
            <CardTitle>Бронирования и заметки</CardTitle>
            <CardDescription>
              Внутренние заметки по брони видны только команде заведения.
            </CardDescription>
          </CardHeader>
          <CardContent>
            {bookingsLoading ? (
              <div className="flex items-center gap-2 py-8 text-muted-foreground">
                <Loader2 className="h-5 w-5 animate-spin" />
                Загрузка бронирований…
              </div>
            ) : bookings.length === 0 ? (
              <p className="text-sm text-muted-foreground">Бронирований нет.</p>
            ) : (
              <div className="space-y-2">
                {bookings.map((b: Booking) => (
                  <Collapsible
                    key={b.id}
                    open={expandedBookingId === b.id}
                    onOpenChange={(open) =>
                      setExpandedBookingId(open ? b.id : null)
                    }
                  >
                    <div className="rounded-lg border border-border">
                      <CollapsibleTrigger asChild>
                        <button
                          type="button"
                          className="flex w-full items-center justify-between gap-2 px-4 py-3 text-left hover:bg-muted/50"
                        >
                          <div className="min-w-0">
                            <div className="font-medium">
                              {b.date} · {b.time}
                            </div>
                            <div className="truncate text-sm text-muted-foreground">
                              {BOOKING_STATUS_LABELS[b.status] ?? b.status} ·{" "}
                              {b.total_price?.toLocaleString("ru-RU")} ₽
                            </div>
                          </div>
                          <ChevronDown
                            className={`h-4 w-4 shrink-0 transition-transform ${
                              expandedBookingId === b.id ? "rotate-180" : ""
                            }`}
                          />
                        </button>
                      </CollapsibleTrigger>
                      <CollapsibleContent>
                        <div className="space-y-3 border-t border-border px-4 py-3">
                          {expandedBookingId === b.id ? (
                            <div className="space-y-3">
                              {notesQueries.isLoading ? (
                                <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                              ) : (
                                <ul className="space-y-2 text-sm">
                                  {(notesQueries.data ?? []).map((n) => (
                                    <li
                                      key={n.id}
                                      className="rounded-md border border-border bg-muted/30 p-2"
                                    >
                                      <div className="text-xs text-muted-foreground">
                                        {formatDt(n.created_at)}
                                      </div>
                                      <p className="mt-1 whitespace-pre-wrap">
                                        {n.body}
                                      </p>
                                    </li>
                                  ))}
                                </ul>
                              )}
                              <div className="flex flex-col gap-2 sm:flex-row">
                                <Textarea
                                  placeholder="Новая заметка…"
                                  className="min-h-[72px] flex-1"
                                  value={noteDrafts[b.id] ?? ""}
                                  onChange={(e) =>
                                    setNoteDrafts((d) => ({
                                      ...d,
                                      [b.id]: e.target.value,
                                    }))
                                  }
                                />
                                <Button
                                  type="button"
                                  className="shrink-0 sm:self-end"
                                  disabled={
                                    addNoteMu.isPending ||
                                    !(noteDrafts[b.id] ?? "").trim()
                                  }
                                  onClick={() =>
                                    addNoteMu.mutate({
                                      bookingId: b.id,
                                      body: (noteDrafts[b.id] ?? "").trim(),
                                    })
                                  }
                                >
                                  Сохранить заметку
                                </Button>
                              </div>
                              <BookingChatPanel
                                kind="venue_booking"
                                refId={b.id}
                                title="Чат с клиентом"
                              />
                            </div>
                          ) : null}
                        </div>
                      </CollapsibleContent>
                    </div>
                  </Collapsible>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </section>
  );
}
