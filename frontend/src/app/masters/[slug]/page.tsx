"use client";

import { use, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
import { Badge } from "@/components/ui/badge";
import { useAuthStore } from "@/store/auth";
import {
  getPublicMaster,
  createMasterBooking,
  formatApiErrorMessage,
  masterCardImageSrc,
  venueMediaUrl,
} from "@/lib/api";
import type { MasterPhoto } from "@/lib/types";
import { ArrowLeft } from "lucide-react";

function sortMasterPhotosPublic(photos?: MasterPhoto[]): MasterPhoto[] {
  if (!photos?.length) return [];
  return [...photos].sort(
    (a, b) => a.sort_order - b.sort_order || a.id.localeCompare(b.id),
  );
}

function kopecksToRub(k: number) {
  return (k / 100).toFixed(0);
}

export default function MasterPublicPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = use(params);
  const router = useRouter();
  const qc = useQueryClient();
  const { token, hydrated } = useAuthStore();
  const [date, setDate] = useState("");
  const [timeFrom, setTimeFrom] = useState("10:00");
  const [timeTo, setTimeTo] = useState("12:00");
  const [comment, setComment] = useState("");
  const [serviceId, setServiceId] = useState<string>("");
  const [msg, setMsg] = useState("");

  const { data: master, isLoading, error } = useQuery({
    queryKey: ["public-master", slug],
    queryFn: () => getPublicMaster(slug),
  });

  const bookMut = useMutation({
    mutationFn: () =>
      createMasterBooking(slug, {
        date,
        time_from: timeFrom,
        time_to: timeTo,
        comment: comment.trim(),
        ...(serviceId ? { master_service_id: serviceId } : {}),
      }),
    onSuccess: () => {
      setMsg("Заявка отправлена. Мастер свяжется с вами.");
      qc.invalidateQueries({ queryKey: ["my-master-bookings"] });
    },
    onError: (e) => setMsg(formatApiErrorMessage(e, "Ошибка")),
  });

  const onBook = () => {
    setMsg("");
    if (!hydrated || !token) {
      router.push(`/auth/login?next=/masters/${encodeURIComponent(slug)}`);
      return;
    }
    if (!date) {
      setMsg("Укажите дату");
      return;
    }
    bookMut.mutate();
  };

  if (isLoading) {
    return (
      <div className="container mx-auto px-4 py-16 text-muted-foreground">Загрузка...</div>
    );
  }

  if (error || !master) {
    return (
      <div className="container mx-auto px-4 py-16">
        <p className="text-destructive">Мастер не найден или профиль не опубликован.</p>
        <Button asChild variant="link" className="mt-4">
          <Link href="/masters">К каталогу</Link>
        </Button>
      </div>
    );
  }

  const gallery = sortMasterPhotosPublic(master.photos);
  const coverSrc = masterCardImageSrc(master);

  return (
    <div className="container mx-auto max-w-3xl px-4 py-10">
      <Button variant="ghost" asChild className="mb-6 gap-2">
        <Link href="/masters">
          <ArrowLeft className="h-4 w-4" />
          Все мастера
        </Link>
      </Button>

      {coverSrc ? (
        <div className="relative mb-8 aspect-[16/10] w-full overflow-hidden rounded-xl border border-border bg-muted">
          <Image
            src={coverSrc}
            alt=""
            fill
            className="object-cover"
            sizes="(max-width: 768px) 100vw, 42rem"
            priority
            unoptimized
          />
        </div>
      ) : null}

      {gallery.length > 1 ? (
        <div className="mb-8 grid grid-cols-3 gap-2 sm:grid-cols-4 md:grid-cols-5">
          {gallery.map((p) => (
            <div
              key={p.id}
              className="relative aspect-square overflow-hidden rounded-lg border border-border bg-muted"
            >
              <Image
                src={venueMediaUrl(p.url)}
                alt=""
                fill
                className="object-cover"
                sizes="120px"
                unoptimized
              />
            </div>
          ))}
        </div>
      ) : null}

      <div className="mb-8">
        <h1 className="text-3xl font-bold mb-2">{master.display_name}</h1>
        <div className="flex flex-wrap gap-2 text-sm text-muted-foreground">
          <span>{master.city}</span>
          {master.experience_years > 0 && (
            <Badge variant="outline">Опыт {master.experience_years} лет</Badge>
          )}
          <Badge variant="secondary">
            {master.work_format === "mobile"
              ? "Выезд"
              : master.work_format === "venue"
                ? "У заведения"
                : "Выезд и у заведения"}
          </Badge>
        </div>
        {master.hourly_rate > 0 && (
          <p className="mt-2 text-lg">
            от {kopecksToRub(master.hourly_rate)} ₽ / час
          </p>
        )}
      </div>

      <Card className="mb-8">
        <CardHeader>
          <CardTitle className="text-lg">О мастере</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4 whitespace-pre-wrap text-muted-foreground">
          <p>{master.bio}</p>
          {master.specializations?.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {master.specializations.map((s) => (
                <Badge key={s} variant="outline">
                  {s}
                </Badge>
              ))}
            </div>
          )}
          {master.phone && (
            <p className="text-foreground">
              Телефон: <a href={`tel:${master.phone}`} className="underline">{master.phone}</a>
            </p>
          )}
        </CardContent>
      </Card>

      {master.services?.length > 0 && (
        <Card className="mb-8">
          <CardHeader>
            <CardTitle className="text-lg">Услуги</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {master.services.map((s) => (
              <div key={s.id} className="flex justify-between gap-4 border-b border-border pb-3 last:border-0">
                <div>
                  <p className="font-medium">{s.name}</p>
                  <p className="text-sm text-muted-foreground">{s.description}</p>
                  <p className="text-xs text-muted-foreground mt-1">{s.duration_min} мин</p>
                </div>
                <p className="shrink-0 font-medium">{kopecksToRub(s.price)} ₽</p>
              </div>
            ))}
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Оставить заявку</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {master.services?.length > 0 && (
            <div className="space-y-2">
              <Label>Услуга (необязательно)</Label>
              <Select value={serviceId || "none"} onValueChange={(v) => setServiceId(v === "none" ? "" : v)}>
                <SelectTrigger>
                  <SelectValue placeholder="Любая" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">Не указывать</SelectItem>
                  {master.services.map((s) => (
                    <SelectItem key={s.id} value={s.id}>
                      {s.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="space-y-2">
              <Label>Дата</Label>
              <Input type="date" value={date} onChange={(e) => setDate(e.target.value)} />
            </div>
            <div className="space-y-2">
              <Label>С</Label>
              <Input type="time" value={timeFrom} onChange={(e) => setTimeFrom(e.target.value)} />
            </div>
            <div className="space-y-2">
              <Label>До</Label>
              <Input type="time" value={timeTo} onChange={(e) => setTimeTo(e.target.value)} />
            </div>
          </div>
          <div className="space-y-2">
            <Label>Комментарий</Label>
            <Textarea value={comment} onChange={(e) => setComment(e.target.value)} rows={3} />
          </div>
          {msg && (
            <p className={msg.startsWith("Заявка") ? "text-green-600 text-sm" : "text-destructive text-sm"}>
              {msg}
            </p>
          )}
          <Button onClick={onBook} disabled={bookMut.isPending}>
            {bookMut.isPending ? "Отправка..." : token ? "Отправить заявку" : "Войти и отправить"}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
