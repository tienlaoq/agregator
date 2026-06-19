"use client";

import { useEffect, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAuthStore } from "@/store/auth";
import {
  getAdminMasters,
  moderateMaster,
  getMasterModerationHistory,
  venueMediaUrl,
} from "@/lib/api";
import { MasterTravelBaseMap } from "@/components/banya/master-travel-base-map";
import type { MasterPhoto, MasterProfile, MasterTravelExcludeZone } from "@/lib/types";
import {
  MASTER_PROFILE_STATUS_LABELS,
  PAYOUT_LEGAL_FORM_LABELS,
  MASTER_CREDENTIAL_KIND_LABELS,
} from "@/lib/types";
import {
  ChevronDown,
  Shield,
  Users,
  History,
  MapPin,
  Phone,
  Clock,
  ExternalLink,
  CheckCircle2,
  XCircle,
  Pause,
  PencilLine,
} from "lucide-react";

const WORK_FORMAT_LABELS: Record<string, string> = {
  venue: "В бане / у заведения",
  mobile: "Выезд к клиенту",
  both: "И то и другое",
};

const STATUS_COLORS: Record<string, string> = {
  draft: "bg-slate-100 text-slate-800",
  pending_review: "bg-amber-100 text-amber-800",
  needs_revision: "bg-orange-100 text-orange-800",
  active: "bg-green-100 text-green-800",
  rejected: "bg-red-100 text-red-800",
  suspended: "bg-gray-100 text-gray-800",
};

type ModerationAction = "approve" | "request_revision" | "reject" | "suspend";

const MODERATION_ACTION_LABELS: Record<ModerationAction, string> = {
  approve: "Одобрить",
  request_revision: "Запросить правки",
  reject: "Отклонить",
  suspend: "Приостановить",
};

function sortMasterPhotos(photos: MasterProfile["photos"]): MasterPhoto[] {
  if (!photos?.length) return [];
  return [...photos].sort(
    (a, b) => a.sort_order - b.sort_order || a.id.localeCompare(b.id),
  );
}

function availabilityNoteFromJson(raw: string | undefined): string {
  if (!raw?.trim() || raw.trim() === "{}") return "";
  try {
    const o = JSON.parse(raw) as { note?: unknown };
    if (o && typeof o === "object" && typeof o.note === "string") return o.note;
  } catch {
    return "";
  }
  return "";
}

function rubFromKopecks(k: number): string {
  if (!Number.isFinite(k)) return "—";
  return new Intl.NumberFormat("ru-RU", {
    maximumFractionDigits: k % 100 === 0 ? 0 : 2,
  }).format(k / 100);
}

function MasterModerationFullProfile({ m }: { m: MasterProfile }) {
  const specs = m.specializations?.filter(Boolean) ?? [];
  const services = [...(m.services ?? [])].sort(
    (a, b) => a.sort_order - b.sort_order || a.name.localeCompare(b.name),
  );
  const photos = sortMasterPhotos(m.photos);
  const availNote = availabilityNoteFromJson(m.availability_json);
  const wf = WORK_FORMAT_LABELS[m.work_format] ?? m.work_format;
  const yandexMapsApiKey = process.env.NEXT_PUBLIC_YANDEX_MAPS_API_KEY;
  const canShowTravelMap =
    (m.work_format === "mobile" || m.work_format === "both") &&
    Number.isFinite(Number(m.travel_radius_km)) &&
    Number(m.travel_radius_km) > 0 &&
    m.travel_base_latitude != null &&
    m.travel_base_longitude != null;
  const excludeZones: MasterTravelExcludeZone[] = (m.travel_exclude_zones ?? []).map((z, i) => ({
    id: z.id?.trim() ? z.id : `ex-${i}`,
    latitude: z.latitude,
    longitude: z.longitude,
    radius_km: z.radius_km,
    label: z.label ?? "",
  }));
  const mapVersion = `${m.id}-${m.updated_at ?? ""}-${excludeZones
    .map((z) => `${z.id}:${z.latitude}:${z.longitude}:${z.radius_km}`)
    .join("|")}`;

  return (
    <details className="group rounded-lg border border-border bg-muted/30">
      <summary className="flex cursor-pointer list-none items-center gap-2 px-3 py-2.5 text-sm font-medium text-foreground hover:bg-muted/50 [&::-webkit-details-marker]:hidden">
        <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-180" />
        Вся информация из анкеты
      </summary>
      <div className="space-y-5 border-t border-border px-3 pb-4 pt-3 text-sm">
        {m.moderation_comment ? (
          <section>
            <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              Последний комментарий модерации
            </h3>
            <p className="whitespace-pre-wrap rounded-md bg-amber-50/80 px-3 py-2 text-foreground dark:bg-amber-950/30">
              {m.moderation_comment}
            </p>
          </section>
        ) : null}

        <section className="space-y-2">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Идентификация
          </h3>
          <dl className="grid gap-3 sm:grid-cols-2">
            <div>
              <dt className="text-xs text-muted-foreground">ID профиля</dt>
              <dd className="mt-0.5 font-mono text-xs break-all">{m.id}</dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">User ID</dt>
              <dd className="mt-0.5 font-mono text-xs break-all">{m.user_id}</dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Слаг</dt>
              <dd className="mt-0.5">{m.slug}</dd>
            </div>
            {m.created_at ? (
              <div>
                <dt className="text-xs text-muted-foreground">Профиль создан</dt>
                <dd className="mt-0.5">{m.created_at}</dd>
              </div>
            ) : null}
            {m.updated_at ? (
              <div>
                <dt className="text-xs text-muted-foreground">Обновлён</dt>
                <dd className="mt-0.5">{m.updated_at}</dd>
              </div>
            ) : null}
            {m.moderated_at ? (
              <div>
                <dt className="text-xs text-muted-foreground">Последняя модерация</dt>
                <dd className="mt-0.5">{m.moderated_at}</dd>
              </div>
            ) : null}
          </dl>
        </section>

        <section className="space-y-2">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            О себе и контакты
          </h3>
          <div>
            <p className="text-xs text-muted-foreground">О себе</p>
            <p className="mt-1 whitespace-pre-wrap text-foreground">{m.bio?.trim() || "—"}</p>
          </div>
          <dl className="grid gap-3 sm:grid-cols-2">
            <div>
              <dt className="text-xs text-muted-foreground">Телефон</dt>
              <dd className="mt-0.5">{m.phone?.trim() || "—"}</dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Город</dt>
              <dd className="mt-0.5">{m.city?.trim() || "—"}</dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Форма выплат</dt>
              <dd className="mt-0.5">
                {m.payout_legal_form
                  ? (PAYOUT_LEGAL_FORM_LABELS[m.payout_legal_form] ?? m.payout_legal_form)
                  : "—"}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Формат работы</dt>
              <dd className="mt-0.5">{wf || "—"}</dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Зона от метки, км</dt>
              <dd className="mt-0.5">{m.travel_radius_km ?? "—"}</dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Опыт, лет</dt>
              <dd className="mt-0.5">{m.experience_years ?? "—"}</dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Базовая ставка</dt>
              <dd className="mt-0.5">
                {m.hourly_rate != null ? `${rubFromKopecks(m.hourly_rate)} ₽/час` : "—"}
              </dd>
            </div>
          </dl>
          {specs.length > 0 ? (
            <div>
              <p className="text-xs text-muted-foreground">Специализации</p>
              <div className="mt-1.5 flex flex-wrap gap-1.5">
                {specs.map((s) => (
                  <Badge key={s} variant="secondary" className="font-normal">
                    {s}
                  </Badge>
                ))}
              </div>
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">Специализации не указаны</p>
          )}
          {availNote ? (
            <div>
              <p className="text-xs text-muted-foreground">Когда на связи / график</p>
              <p className="mt-1 whitespace-pre-wrap text-foreground">{availNote}</p>
            </div>
          ) : null}
        </section>

        <section className="space-y-2">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Услуги ({services.length})
          </h3>
          {services.length === 0 ? (
            <p className="text-muted-foreground">Нет услуг</p>
          ) : (
            <ul className="space-y-3 rounded-md border border-border bg-background p-3">
              {services.map((s) => (
                <li key={s.id} className="border-b border-border pb-3 last:border-0 last:pb-0">
                  <p className="font-medium">{s.name || "—"}</p>
                  {s.description?.trim() ? (
                    <p className="mt-1 whitespace-pre-wrap text-muted-foreground">{s.description}</p>
                  ) : null}
                  <p className="mt-1 text-xs text-muted-foreground">
                    {s.duration_min} мин · {rubFromKopecks(s.price)} ₽
                  </p>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section className="space-y-2">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Сертификаты и награды ({m.credentials?.length ?? 0})
          </h3>
          {!m.credentials?.length ? (
            <p className="text-muted-foreground">Не указаны</p>
          ) : (
            <ul className="space-y-2 rounded-md border border-border bg-background p-3">
              {[...m.credentials]
                .sort((a, b) => a.sort_order - b.sort_order)
                .map((c) => (
                  <li key={c.id} className="border-b border-border pb-2 last:border-0 last:pb-0">
                    <p className="font-medium">{c.title}</p>
                    <p className="text-xs text-muted-foreground">
                      {MASTER_CREDENTIAL_KIND_LABELS[c.kind] ?? c.kind}
                      {c.issuer ? ` · ${c.issuer}` : ""}
                      {c.year && c.year > 0 ? ` · ${c.year}` : ""}
                    </p>
                  </li>
                ))}
            </ul>
          )}
        </section>

        <section className="space-y-2">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Яндекс.Карты и зона выезда
          </h3>
          <dl className="grid gap-3 sm:grid-cols-2">
            <div>
              <dt className="text-xs text-muted-foreground">Точка отсчёта (lat, lon)</dt>
              <dd className="mt-0.5 font-mono text-xs">
                {m.travel_base_latitude != null && m.travel_base_longitude != null
                  ? `${m.travel_base_latitude.toFixed(6)}, ${m.travel_base_longitude.toFixed(6)}`
                  : "—"}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Ссылка в Яндекс.Карты</dt>
              <dd className="mt-0.5">
                {m.travel_base_latitude != null && m.travel_base_longitude != null ? (
                  <a
                    href={`https://yandex.ru/maps/?pt=${m.travel_base_longitude},${m.travel_base_latitude}&z=14&l=map`}
                    target="_blank"
                    rel="noreferrer"
                    className="text-primary underline"
                  >
                    Открыть метку
                  </a>
                ) : (
                  "—"
                )}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Радиус зоны, км</dt>
              <dd className="mt-0.5">{m.travel_radius_km ?? "—"}</dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Исключающие зоны</dt>
              <dd className="mt-0.5">{excludeZones.length}</dd>
            </div>
          </dl>
          {canShowTravelMap ? (
            yandexMapsApiKey ? (
              <MasterTravelBaseMap
                readOnly
                apiKey={yandexMapsApiKey}
                mapVersion={mapVersion}
                cityHint={m.city || ""}
                seedLat={m.travel_base_latitude ?? null}
                seedLon={m.travel_base_longitude ?? null}
                onPositionChange={() => {}}
                travelRadiusKm={Number(m.travel_radius_km) || 0}
                excludeZones={excludeZones}
              />
            ) : (
              <p className="text-xs text-muted-foreground">
                Карта не отображается: не задан `NEXT_PUBLIC_YANDEX_MAPS_API_KEY`.
              </p>
            )
          ) : (
            <p className="text-xs text-muted-foreground">
              Для этого мастера зона выезда на карте не используется или заполнена не полностью.
            </p>
          )}
        </section>

        <section className="space-y-2">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Фотографии ({photos.length})
          </h3>
          {photos.length === 0 ? (
            <p className="text-muted-foreground">Нет фото</p>
          ) : (
            <div className="flex flex-wrap gap-3">
              {photos.map((p) => (
                <div
                  key={p.id}
                  className="relative h-28 w-28 overflow-hidden rounded-md border border-border bg-muted"
                >
                  <Image
                    src={venueMediaUrl(p.url)}
                    alt=""
                    fill
                    className="object-cover"
                    sizes="112px"
                    unoptimized
                  />
                  {p.is_cover ? (
                    <span className="absolute left-1 top-1 rounded bg-background/90 px-1.5 py-0.5 text-[10px] font-medium">
                      Обложка
                    </span>
                  ) : null}
                </div>
              ))}
            </div>
          )}
        </section>
      </div>
    </details>
  );
}

export default function AdminMastersPage() {
  const router = useRouter();
  const qc = useQueryClient();
  const { token, user, hydrated } = useAuthStore();
  const [filterStatus, setFilterStatus] = useState("pending_review");
  const [comment, setComment] = useState("");
  const [pending, setPending] = useState<{ id: string; action: ModerationAction } | null>(null);
  const [expandedId, setExpandedId] = useState<string | null>(null);

  useEffect(() => {
    if (hydrated && (!token || user?.role !== "admin")) {
      router.push("/");
    }
  }, [hydrated, token, user, router]);

  const { data, isLoading } = useQuery({
    queryKey: ["admin-masters", filterStatus],
    queryFn: () => getAdminMasters({ status: filterStatus, limit: 100 }),
    enabled: !!token && user?.role === "admin",
  });

  const historyQuery = useQuery({
    queryKey: ["admin-master-history", expandedId],
    queryFn: () => getMasterModerationHistory(expandedId!, 50),
    enabled: !!expandedId && user?.role === "admin",
  });

  const mutation = useMutation({
    mutationFn: ({
      id,
      action,
      comment: c,
    }: {
      id: string;
      action: ModerationAction;
      comment: string;
    }) => moderateMaster(id, action, c),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin-masters"] });
      setComment("");
      setPending(null);
    },
  });

  const masters = data?.masters ?? [];

  const needComment = (action: ModerationAction) =>
    action === "request_revision" || action === "reject" || action === "suspend";

  // Действия без комментария (одобрить) выполняются сразу; остальным нужен
  // комментарий — раскрываем поле прямо в карточке и ждём подтверждения.
  const start = (id: string, action: ModerationAction) => {
    if (!needComment(action)) {
      mutation.mutate({ id, action, comment: "" });
      return;
    }
    setPending({ id, action });
    setComment("");
  };

  const confirm = () => {
    if (!pending || !comment.trim()) return;
    mutation.mutate({ id: pending.id, action: pending.action, comment: comment.trim() });
  };

  const cancel = () => {
    setPending(null);
    setComment("");
  };

  if (!hydrated || !token || user?.role !== "admin") return null;

  return (
    <div className="mx-auto max-w-4xl px-4 py-8">
      <div className="mb-6 flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center gap-2">
          <Users className="h-7 w-7" />
          <h1 className="text-2xl font-bold">Модерация мастеров</h1>
        </div>
        <Select value={filterStatus} onValueChange={setFilterStatus}>
          <SelectTrigger className="w-[220px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="draft">Черновики</SelectItem>
            <SelectItem value="pending_review">На проверке</SelectItem>
            <SelectItem value="needs_revision">Нужны правки</SelectItem>
            <SelectItem value="active">Активные</SelectItem>
            <SelectItem value="rejected">Отклонённые</SelectItem>
            <SelectItem value="suspended">Приостановленные</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {isLoading && <p className="text-muted-foreground">Загрузка...</p>}

      {!isLoading && masters.length === 0 && (
        <p className="text-muted-foreground">Нет записей в этом статусе.</p>
      )}

      <div className="space-y-4">
        {masters.map((m) => {
          const meta = MASTER_PROFILE_STATUS_LABELS[m.status];
          return (
            <Card key={m.id} className="border-border">
              <CardHeader className="pb-3">
                <div className="flex items-start justify-between gap-2">
                  <div>
                    <CardTitle className="text-lg">{m.display_name}</CardTitle>
                    <div className="mt-1 flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
                      {m.work_format && (
                        <Badge variant="outline" className="text-xs">
                          {WORK_FORMAT_LABELS[m.work_format] ?? m.work_format}
                        </Badge>
                      )}
                      <span className="flex items-center gap-1">
                        <MapPin className="h-3.5 w-3.5" />
                        {m.city || "Город не указан"}
                      </span>
                      {m.phone && (
                        <span className="flex items-center gap-1">
                          <Phone className="h-3.5 w-3.5" />
                          {m.phone}
                        </span>
                      )}
                      {m.created_at && (
                        <span className="flex items-center gap-1">
                          <Clock className="h-3.5 w-3.5" />
                          {new Date(m.created_at).toLocaleDateString("ru-RU")}
                        </span>
                      )}
                      <span className="font-mono text-xs">{m.slug}</span>
                    </div>
                  </div>
                  <Badge className={STATUS_COLORS[m.status] ?? STATUS_COLORS.pending_review}>
                    {meta?.label ?? m.status}
                  </Badge>
                </div>
              </CardHeader>
              <CardContent className="space-y-3">
                <p className="text-sm text-muted-foreground">{m.bio?.trim() || "О себе не заполнено"}</p>

                <MasterModerationFullProfile m={m} />

                <div className="rounded-md border border-primary/20 bg-primary/5 p-3 text-sm">
                  <div className="mb-2 flex items-center gap-2 font-medium text-foreground">
                    <Shield className="h-4 w-4 text-primary" />
                    Проверка выплат
                  </div>
                  {m.payout_legal_form ? (
                    <p>
                      <span className="text-muted-foreground">Форма выплат: </span>
                      {PAYOUT_LEGAL_FORM_LABELS[m.payout_legal_form] ?? m.payout_legal_form}
                    </p>
                  ) : (
                    <p className="text-amber-700 dark:text-amber-500">Форма выплат не указана</p>
                  )}
                  {m.payout_legal_name && (
                    <p className="mt-1">
                      <span className="text-muted-foreground">Юр. наименование: </span>
                      {m.payout_legal_name}
                    </p>
                  )}
                  {(m.payout_inn || m.payout_ogrn || m.payout_ogrnip) && (
                    <p className="mt-1 font-mono text-xs">
                      {m.payout_inn && <>ИНН: {m.payout_inn}</>}
                      {m.payout_inn && (m.payout_ogrn || m.payout_ogrnip) && " · "}
                      {(m.payout_ogrn || m.payout_ogrnip) && (
                        <>ОГРН/ОГРНИП: {m.payout_ogrn || m.payout_ogrnip}</>
                      )}
                    </p>
                  )}
                </div>

                {m.moderation_comment && (
                  <div className="rounded-md bg-muted p-3 text-sm">
                    <span className="font-medium">Комментарий модератора: </span>
                    {m.moderation_comment}
                  </div>
                )}

                {pending?.id === m.id && (
                  <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-3">
                    <div className="flex items-center gap-2 text-sm font-medium">
                      <Shield className="h-4 w-4" />
                      Комментарий к решению «{MODERATION_ACTION_LABELS[pending.action]}»
                    </div>
                    <Textarea
                      autoFocus
                      value={comment}
                      onChange={(e) => setComment(e.target.value)}
                      placeholder="Обязателен для «Запросить правки», «Отклонить», «Приостановить»"
                      rows={3}
                    />
                    <div className="flex gap-2">
                      <Button
                        size="sm"
                        disabled={!comment.trim() || mutation.isPending}
                        onClick={confirm}
                      >
                        Подтвердить
                      </Button>
                      <Button size="sm" variant="ghost" onClick={cancel}>
                        Отмена
                      </Button>
                    </div>
                  </div>
                )}

                {pending?.id !== m.id && (
                  <div className="flex flex-wrap gap-2">
                    {m.status === "active" && m.slug && (
                      <Button size="sm" variant="outline" className="gap-1.5" asChild>
                        <Link href={`/masters/${m.slug}`} target="_blank" rel="noopener noreferrer">
                          <ExternalLink className="h-4 w-4" />
                          Просмотр карточки
                        </Link>
                      </Button>
                    )}
                    {m.status === "pending_review" && (
                      <>
                        <Button
                          size="sm"
                          className="gap-1.5 bg-green-600 hover:bg-green-700"
                          disabled={mutation.isPending}
                          onClick={() => start(m.id, "approve")}
                        >
                          <CheckCircle2 className="h-4 w-4" />
                          Одобрить
                        </Button>
                        <Button
                          size="sm"
                          variant="secondary"
                          className="gap-1.5"
                          disabled={mutation.isPending}
                          onClick={() => start(m.id, "request_revision")}
                        >
                          <PencilLine className="h-4 w-4" />
                          Запросить правки
                        </Button>
                        <Button
                          size="sm"
                          variant="destructive"
                          className="gap-1.5"
                          disabled={mutation.isPending}
                          onClick={() => start(m.id, "reject")}
                        >
                          <XCircle className="h-4 w-4" />
                          Отклонить
                        </Button>
                      </>
                    )}
                    {m.status === "active" && (
                      <Button
                        size="sm"
                        variant="destructive"
                        className="gap-1.5"
                        disabled={mutation.isPending}
                        onClick={() => start(m.id, "suspend")}
                      >
                        <Pause className="h-4 w-4" />
                        Приостановить
                      </Button>
                    )}
                    {(m.status === "suspended" || m.status === "needs_revision") && (
                      <>
                        <Button
                          size="sm"
                          className="gap-1.5 bg-green-600 hover:bg-green-700"
                          disabled={mutation.isPending}
                          onClick={() => start(m.id, "approve")}
                        >
                          <CheckCircle2 className="h-4 w-4" />
                          Одобрить / вернуть в каталог
                        </Button>
                        <Button
                          size="sm"
                          variant="secondary"
                          className="gap-1.5"
                          disabled={mutation.isPending}
                          onClick={() => start(m.id, "request_revision")}
                        >
                          <PencilLine className="h-4 w-4" />
                          Снова запросить правки
                        </Button>
                      </>
                    )}
                    <Button
                      size="sm"
                      variant="outline"
                      className="gap-1.5"
                      onClick={() => setExpandedId(expandedId === m.id ? null : m.id)}
                    >
                      <History className="h-3.5 w-3.5" />
                      История
                    </Button>
                  </div>
                )}
                {expandedId === m.id && historyQuery.data && (
                  <ul className="mt-2 space-y-2 border-t border-border pt-3 text-xs">
                    {historyQuery.data.entries.map((e) => (
                      <li key={e.id} className="text-muted-foreground">
                        <span className="font-medium text-foreground">
                          {e.old_status} → {e.new_status}
                        </span>{" "}
                        · {e.created_at}
                        {e.comment && <span className="block mt-1">{e.comment}</span>}
                      </li>
                    ))}
                  </ul>
                )}
              </CardContent>
            </Card>
          );
        })}
      </div>
    </div>
  );
}
