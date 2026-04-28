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
import type { MasterPhoto, MasterProfile } from "@/lib/types";
import { MASTER_PROFILE_STATUS_LABELS, PAYOUT_LEGAL_FORM_LABELS } from "@/lib/types";
import { ChevronDown, Shield, Users, History } from "lucide-react";

const WORK_FORMAT_LABELS: Record<string, string> = {
  venue: "В бане / у заведения",
  mobile: "Выезд к клиенту",
  both: "И то и другое",
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
      action: "approve" | "request_revision" | "reject" | "suspend";
      comment: string;
    }) => moderateMaster(id, action, c),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin-masters"] });
      setComment("");
    },
  });

  const masters = data?.masters ?? [];

  const needComment = (action: string) =>
    action === "request_revision" || action === "reject" || action === "suspend";

  const run = (id: string, action: "approve" | "request_revision" | "reject" | "suspend") => {
    if (needComment(action) && !comment.trim()) {
      alert("Укажите комментарий для этого решения");
      return;
    }
    mutation.mutate({ id, action, comment: comment.trim() });
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

      <Card className="mb-6">
        <CardHeader className="py-3">
          <CardTitle className="text-sm font-medium flex items-center gap-2">
            <Shield className="h-4 w-4" />
            Комментарий к решению
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Textarea
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            placeholder="Обязателен для «Запросить правки», «Отклонить», «Приостановить»"
            rows={3}
          />
        </CardContent>
      </Card>

      {isLoading && <p className="text-muted-foreground">Загрузка...</p>}

      {!isLoading && masters.length === 0 && (
        <p className="text-muted-foreground">Нет записей в этом статусе.</p>
      )}

      <div className="space-y-4">
        {masters.map((m) => {
          const meta = MASTER_PROFILE_STATUS_LABELS[m.status];
          return (
            <Card key={m.id}>
              <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-2">
                <div>
                  <CardTitle className="text-lg">{m.display_name}</CardTitle>
                  <p className="text-sm text-muted-foreground">
                    {m.city} · {m.slug}
                  </p>
                  <Badge className="mt-2" variant="secondary">
                    {meta?.label ?? m.status}
                  </Badge>
                </div>
                {m.status === "active" ? (
                  <Button variant="outline" size="sm" asChild>
                    <Link href={`/masters/${m.slug}`} target="_blank">
                      Публичная страница
                    </Link>
                  </Button>
                ) : (
                  <span className="text-xs text-muted-foreground">Не в каталоге</span>
                )}
              </CardHeader>
              <CardContent className="space-y-3 text-sm">
                <p className="text-muted-foreground line-clamp-2">{m.bio || "О себе не заполнено"}</p>
                <p>
                  <span className="text-muted-foreground">Телефон:</span> {m.phone || "—"}
                </p>
                {m.payout_legal_form ? (
                  <p>
                    <span className="text-muted-foreground">Выплаты:</span>{" "}
                    {PAYOUT_LEGAL_FORM_LABELS[m.payout_legal_form] ?? m.payout_legal_form}
                  </p>
                ) : (
                  <p className="text-amber-700 dark:text-amber-500">Форма выплат не указана</p>
                )}
                <MasterModerationFullProfile m={m} />
                <div className="flex flex-wrap gap-2 pt-2">
                  {m.status === "pending_review" && (
                    <>
                      <Button size="sm" onClick={() => run(m.id, "approve")}>
                        Одобрить
                      </Button>
                      <Button size="sm" variant="secondary" onClick={() => run(m.id, "request_revision")}>
                        Запросить правки
                      </Button>
                      <Button size="sm" variant="destructive" onClick={() => run(m.id, "reject")}>
                        Отклонить
                      </Button>
                    </>
                  )}
                  {m.status === "active" && (
                    <Button size="sm" variant="destructive" onClick={() => run(m.id, "suspend")}>
                      Приостановить
                    </Button>
                  )}
                  {(m.status === "suspended" || m.status === "needs_revision") && (
                    <>
                      <Button size="sm" onClick={() => run(m.id, "approve")}>
                        Одобрить / вернуть в каталог
                      </Button>
                      <Button size="sm" variant="secondary" onClick={() => run(m.id, "request_revision")}>
                        Снова запросить правки
                      </Button>
                    </>
                  )}
                  <Button
                    size="sm"
                    variant="outline"
                    className="gap-1"
                    onClick={() => setExpandedId(expandedId === m.id ? null : m.id)}
                  >
                    <History className="h-3 w-3" />
                    История
                  </Button>
                </div>
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
