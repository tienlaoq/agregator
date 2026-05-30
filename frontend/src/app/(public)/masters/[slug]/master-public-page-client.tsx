"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useGallery } from "@/hooks/use-gallery";
import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { format, isBefore, startOfDay } from "date-fns";
import { ru } from "date-fns/locale";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Calendar } from "@/components/ui/calendar";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { useAuthStore } from "@/store/auth";
import {
  getPublicMaster,
  createMasterBooking,
  formatApiErrorMessage,
  getMasterReviews,
  masterCardImageSrc,
  masterCardPriceLabel,
  venueMediaUrl,
} from "@/lib/api";
import { ReviewList } from "@/components/review-list";
import type { MasterPhoto, MasterProfile, MasterTravelExcludeZone, Review } from "@/lib/types";
import { MasterTravelBaseMap } from "@/components/banya/master-travel-base-map";
import {
  defaultEndTimeForDuration,
  endTimeOptionsThirtyMinutes,
  slotLengthMinutes,
  thirtyMinuteStartGridFrom10To22,
} from "@/lib/booking-slot-time";
import {
  ArrowLeft,
  CalendarIcon,
  Check,
  ChevronLeft,
  ChevronRight,
  ImageIcon,
  MapPin,
  Minus,
  Phone,
  Plus,
  Users,
} from "lucide-react";

function sortMasterPhotosPublic(photos?: MasterPhoto[]): MasterPhoto[] {
  if (!photos?.length) return [];
  return [...photos].sort(
    (a, b) => a.sort_order - b.sort_order || a.id.localeCompare(b.id),
  );
}

function kopecksToRub(k: number) {
  return (k / 100).toFixed(0);
}

function masterServiceDurationMin(s: { duration_min: number }): number {
  const d = s.duration_min;
  return d > 0 ? d : 120;
}

export function MasterPublicPageClient({
  slug,
  initialMaster,
}: {
  slug: string;
  initialMaster?: MasterProfile;
}) {
  const router = useRouter();
  const qc = useQueryClient();
  const { token, user, hydrated } = useAuthStore();
  const [date, setDate] = useState<Date>();
  const [time, setTime] = useState("");
  const [timeTo, setTimeTo] = useState("");
  const [selectedServiceIds, setSelectedServiceIds] = useState<string[]>([]);
  const [guests] = useState(1);
  const [comment, setComment] = useState("");
  const [msg, setMsg] = useState("");
  // Gallery state управляется через useGallery ниже
  const [reviews, setReviews] = useState<Review[]>([]);
  const ssrMaster = initialMaster && initialMaster.slug === slug ? initialMaster : undefined;

  const { data: master, isLoading, error } = useQuery({
    queryKey: ["public-master", slug],
    queryFn: () => getPublicMaster(slug),
    initialData: ssrMaster,
    staleTime: ssrMaster ? 60_000 : 0,
    refetchOnMount: ssrMaster ? false : true,
  });

  // Render-time сброс reviews при смене мастера, а сам fetch — в effect
  // (legit async sync с внешним миром). Раньше setReviews([]) синхронно
  // в начале effect — нарушение react-hooks/set-state-in-effect.
  const [prevMasterIdForReviews, setPrevMasterIdForReviews] = useState<string | undefined>(master?.id);
  if (master?.id !== prevMasterIdForReviews) {
    setPrevMasterIdForReviews(master?.id);
    setReviews([]);
  }
  useEffect(() => {
    if (!master?.id) return;
    getMasterReviews(master.id)
      .then(setReviews)
      .catch(() => {
        setReviews([]);
      });
  }, [master?.id]);

  // useCallback здесь был обязан правилам React Compiler удалить (он сам
  // мемоизует). Оставлен как обычная функция — компилятор автоматически
  // делает её стабильной для дочерних компонентов.
  const reloadReviews = async () => {
    if (!master?.id) return;
    try {
      const r = await getMasterReviews(master.id);
      setReviews(r);
    } catch {
      // keep current state on refresh errors
    }
  };

  const showPublicTravelZone = useMemo(() => {
    if (!master) return false;
    const wf = (master.work_format || "").toLowerCase();
    if (wf !== "mobile" && wf !== "both") return false;
    const r = Number(master.travel_radius_km);
    const lat = master.travel_base_latitude;
    const lon = master.travel_base_longitude;
    if (!Number.isFinite(r) || r <= 0) return false;
    if (lat == null || lon == null || !Number.isFinite(lat) || !Number.isFinite(lon)) return false;
    return true;
  }, [master]);

  const publicTravelMapVersion = useMemo(() => {
    if (!master || !showPublicTravelZone) return "0";
    const z = (master.travel_exclude_zones ?? [])
      .map((x) => `${x.id}:${x.latitude}:${x.longitude}:${x.radius_km}`)
      .join("|");
    return `${master.slug}-${master.travel_radius_km}-${master.travel_base_latitude}-${master.travel_base_longitude}-${z}`;
  }, [master, showPublicTravelZone]);

  const publicExcludeZones: MasterTravelExcludeZone[] = useMemo(() => {
    if (!master?.travel_exclude_zones?.length) return [];
    return master.travel_exclude_zones.map((z, i) => ({
      id: z.id?.trim() ? z.id : `ex-${i}`,
      latitude: z.latitude,
      longitude: z.longitude,
      radius_km: z.radius_km,
      label: typeof z.label === "string" ? z.label : "",
    }));
  }, [master]);

  const noopTravelPosition = useCallback(() => {}, []);
  const yandexMapsApiKey = process.env.NEXT_PUBLIC_YANDEX_MAPS_API_KEY;

  const bookMut = useMutation({
    mutationFn: () =>
      createMasterBooking(slug, {
        date: format(date!, "yyyy-MM-dd"),
        time_from: time,
        time_to: timeTo,
        comment: comment.trim(),
        ...(selectedServiceIds.length === 1
          ? { master_service_id: selectedServiceIds[0] }
          : {}),
      }),
    onSuccess: (b) => {
      setMsg("Заявка отправлена. Перенаправляем на оплату...");
      qc.invalidateQueries({ queryKey: ["my-master-bookings"] });
      if (b.payment_url) {
        window.location.assign(b.payment_url);
      }
    },
    onError: (e) => setMsg(formatApiErrorMessage(e, "Ошибка")),
  });

  const slotDurationMin = useMemo(() => {
    if (time && timeTo) {
      const m = slotLengthMinutes(time, timeTo);
      if (m != null && m >= 30 && m <= 720) return m;
    }
    if (selectedServiceIds.length > 0 && master) {
      let m = 30;
      for (const id of selectedServiceIds) {
        const s = master.services?.find((x) => x.id === id);
        if (!s) continue;
        m = Math.max(m, masterServiceDurationMin(s));
      }
      return Math.min(720, Math.max(30, m));
    }
    return 120;
  }, [time, timeTo, selectedServiceIds, master]);

  const slotValid = useMemo(() => {
    if (!time || !timeTo) return false;
    const m = slotLengthMinutes(time, timeTo);
    return m != null && m >= 30 && m <= 720 && m % 30 === 0;
  }, [time, timeTo]);

  const visitEndOptions = useMemo(() => endTimeOptionsThirtyMinutes(time), [time]);
  const startTimeGrid = useMemo(() => thirtyMinuteStartGridFrom10To22(), []);

  const priceHint = useMemo(() => {
    if (!master) return null;
    if (selectedServiceIds.length > 0) {
      let sumRub = 0;
      let hourlySlots = 0;
      for (const id of selectedServiceIds) {
        const s = master.services?.find((x) => x.id === id);
        if (!s) continue;
        if (Number(s.price) > 0) sumRub += Number(s.price) / 100;
        else hourlySlots++;
      }
      if (hourlySlots > 1) return null;
      if (hourlySlots === 1 && typeof master.hourly_rate === "number" && master.hourly_rate > 0 && slotValid) {
        const mins = slotLengthMinutes(time, timeTo);
        if (mins != null) {
          const hours = Math.ceil(mins / 60);
          sumRub += (master.hourly_rate / 100) * hours;
        }
      }
      if (sumRub > 0) {
        return `${new Intl.NumberFormat("ru-RU", {
          minimumFractionDigits: 0,
          maximumFractionDigits: 0,
        }).format(sumRub)} ₽`;
      }
      return null;
    }
    if (typeof master.hourly_rate === "number" && master.hourly_rate > 0 && slotValid) {
      const mins = slotLengthMinutes(time, timeTo);
      if (mins != null) {
        const hours = Math.ceil(mins / 60);
        const rub = new Intl.NumberFormat("ru-RU", {
          minimumFractionDigits: 0,
          maximumFractionDigits: 0,
        }).format((master.hourly_rate / 100) * hours);
        return `≈ ${rub} ₽`;
      }
    }
    return null;
  }, [master, selectedServiceIds, time, timeTo, slotValid]);

  // Render-time управление time/timeTo вместо трёх useEffect'ов
  // (react-hooks/set-state-in-effect). Логика:
  //   1) при сбросе date — очищаем time и timeTo
  //   2) при смене (time | selectedServiceIds | master.id) — пересчитываем timeTo
  //   3) если текущий timeTo не входит в валидные опции для time — пересчитываем
  const computeAutoTimeTo = (): string => {
    if (!time || !master) return "";
    let addMin = 120;
    if (selectedServiceIds.length > 0) {
      let m = 30;
      for (const id of selectedServiceIds) {
        const s = master.services?.find((x) => x.id === id);
        if (!s) continue;
        m = Math.max(m, masterServiceDurationMin(s));
      }
      addMin = Math.max(30, m);
    }
    return defaultEndTimeForDuration(time, addMin);
  };

  // 1) Сброс при очистке date.
  const [prevDate, setPrevDate] = useState(date);
  if (date !== prevDate) {
    setPrevDate(date);
    if (!date) {
      setTime("");
      setTimeTo("");
    }
  }

  // 2) Пересчёт timeTo при смене входов.
  const autoTimeToKey = `${time}|${selectedServiceIds.join(",")}|${master?.id ?? ""}`;
  const [prevAutoTimeToKey, setPrevAutoTimeToKey] = useState(autoTimeToKey);
  if (autoTimeToKey !== prevAutoTimeToKey) {
    setPrevAutoTimeToKey(autoTimeToKey);
    if (!time) {
      setTimeTo("");
    } else if (master) {
      setTimeTo(computeAutoTimeTo());
    }
  }

  // 3) Самокорректирующая проверка: если current timeTo не валиден — заменяем.
  // Render-time setState разрешён, пока выход стабилизируется (newTimeTo === timeTo
  // на следующем рендере).
  if (time && timeTo && master) {
    const opts = endTimeOptionsThirtyMinutes(time);
    if (opts.length > 0 && !opts.includes(timeTo)) {
      const newTimeTo = computeAutoTimeTo();
      if (newTimeTo !== timeTo) {
        setTimeTo(newTimeTo);
      }
    }
  }

  const onBook = () => {
    setMsg("");
    if (!hydrated || !token) {
      router.push(`/auth/login?next=/masters/${encodeURIComponent(slug)}`);
      return;
    }
    if (!date) {
      setMsg("Выберите дату");
      return;
    }
    if (!time || !timeTo || !slotValid) {
      setMsg("Выберите время начала и окончания визита");
      return;
    }
    if (isBefore(startOfDay(date), startOfDay(new Date()))) {
      setMsg("Нельзя выбрать прошедшую дату.");
      return;
    }
    bookMut.mutate();
  };

  const sortedPhotos = sortMasterPhotosPublic(master?.photos);
  const coverSrc = master ? masterCardImageSrc(master) : "";
  const priceLine = master ? masterCardPriceLabel(master) : "";

  // Слайды для галереи: отсортированные фото мастера или обложка как запасной вариант
  const displayPhotos = sortedPhotos.length > 0
    ? sortedPhotos.map((p) => ({ id: p.id, src: venueMediaUrl(p.url) }))
    : coverSrc
      ? [{ id: "cover", src: coverSrc }]
      : [];

  // useGallery берёт на себя: индекс, лайтбокс, клавиатуру, сброс при смене мастера
  const photoGallery = useGallery(displayPhotos, `${slug}-${master?.id ?? ""}`);

  if (isLoading) {
    return (
      <div className="container mx-auto px-4 py-16">
        <div className="grid gap-8 lg:grid-cols-3">
          <div className="lg:col-span-2 space-y-6">
            <div className="h-80 animate-pulse rounded-xl bg-muted" />
            <div className="h-8 w-2/3 animate-pulse rounded bg-muted" />
            <div className="h-4 w-1/2 animate-pulse rounded bg-muted" />
          </div>
          <div className="h-96 animate-pulse rounded-xl bg-muted" />
        </div>
      </div>
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

  return (
    <section className="bg-background py-10 md:py-16">
      <div className="container mx-auto max-w-6xl px-4">
        <Link
          href="/masters"
          className="mb-6 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          Назад к каталогу
        </Link>

        <div className="grid gap-8 lg:grid-cols-3">
          <div className="lg:col-span-2">
            <div className="mb-8">
              {photoGallery.current ? (
                <button
                  type="button"
                  className="relative aspect-[16/9] w-full overflow-hidden rounded-xl border border-border bg-muted"
                  onClick={() => photoGallery.openAt(photoGallery.index)}
                  aria-label="Открыть фото в полном размере"
                >
                  <Image
                    src={photoGallery.current.src}
                    alt=""
                    fill
                    className="object-cover"
                    sizes="(max-width: 1024px) 100vw, 66vw"
                    priority
                    unoptimized
                  />
                </button>
              ) : (
                <div className="flex aspect-[16/9] items-center justify-center rounded-xl border border-border bg-muted">
                  <div className="flex flex-col items-center gap-2 text-muted-foreground/40">
                    <ImageIcon className="h-16 w-16" />
                    <span className="text-sm">Фото ещё не добавлены</span>
                  </div>
                </div>
              )}
              {photoGallery.count > 1 ? (
                <div className="mt-3 flex gap-2 overflow-x-auto pb-1 pt-0.5 [-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
                  {displayPhotos.map((p, idx) => (
                    <button
                      type="button"
                      key={p.id}
                      onClick={() => photoGallery.openAt(idx)}
                      aria-label={`Показать фото ${idx + 1}`}
                      className={`relative h-16 w-16 shrink-0 overflow-hidden rounded-md border bg-muted ${
                        idx === photoGallery.index
                          ? "border-primary ring-2 ring-primary/30"
                          : "border-border"
                      }`}
                    >
                      <Image
                        src={p.src}
                        alt=""
                        fill
                        className="object-cover"
                        sizes="64px"
                        unoptimized
                      />
                    </button>
                  ))}
                </div>
              ) : null}
            </div>

            <div className="mb-8">
              <div className="mb-4 flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
                {master.experience_years > 0 && (
                  <Badge variant="outline">Опыт {master.experience_years} лет</Badge>
                )}
                <Badge variant="secondary">
                  {master.work_format === "mobile"
                    ? "Выезд к клиенту"
                    : master.work_format === "venue"
                      ? "В бане / у заведения"
                      : "И то и другое"}
                </Badge>
              </div>
              <h1 className="mb-4 text-3xl font-bold text-foreground md:text-4xl">{master.display_name}</h1>
              <div className="flex flex-col gap-2 text-muted-foreground">
                <div className="flex items-center gap-2">
                  <MapPin className="h-5 w-5 shrink-0" />
                  {master.city}
                </div>
                {master.phone && (
                  <div className="flex items-center gap-2">
                    <Phone className="h-5 w-5 shrink-0" />
                    <a href={`tel:${master.phone}`} className="hover:underline">
                      {master.phone}
                    </a>
                  </div>
                )}
              </div>
            </div>

            {priceLine ? (
              <div className="mb-8">
                <span className="text-2xl font-bold text-primary">{priceLine}</span>
              </div>
            ) : null}

            {master.bio?.trim() ? (
              <div className="mb-8">
                <h2 className="mb-4 text-xl font-semibold text-foreground">Описание</h2>
                <p className="whitespace-pre-wrap leading-relaxed text-muted-foreground">{master.bio}</p>
                {master.specializations?.length ? (
                  <div className="mt-4 flex flex-wrap gap-2">
                    {master.specializations.map((s) => (
                      <Badge key={s} variant="outline">
                        {s}
                      </Badge>
                    ))}
                  </div>
                ) : null}
              </div>
            ) : null}

            {master.services && master.services.length > 0 ? (
              <div className="mb-8">
                <h2 className="mb-2 text-xl font-semibold text-foreground">Услуги и цены</h2>
                <p className="mb-4 text-sm text-muted-foreground">
                  Нажмите на карточку, чтобы добавить услугу в заявку или убрать её. Можно выбрать несколько услуг одновременно.
                </p>
                <div className="space-y-3">
                  {master.services.map((svc) => {
                    const selected = selectedServiceIds.includes(svc.id);
                    return (
                    <Card
                      key={svc.id}
                      role="button"
                      tabIndex={0}
                      aria-pressed={selected}
                      onClick={() =>
                        setSelectedServiceIds((prev) =>
                          prev.includes(svc.id) ? prev.filter((x) => x !== svc.id) : [...prev, svc.id],
                        )
                      }
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault();
                          setSelectedServiceIds((prev) =>
                            prev.includes(svc.id) ? prev.filter((x) => x !== svc.id) : [...prev, svc.id],
                          );
                        }
                      }}
                      className={`cursor-pointer border-2 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
                        selected ? "border-primary bg-primary/5" : "border-border hover:border-primary/40"
                      }`}
                    >
                      <CardContent className="relative flex items-center justify-between gap-4 p-4 pr-14">
                        {selected ? (
                          <div
                            className="absolute right-3 top-1/2 flex h-8 w-8 -translate-y-1/2 items-center justify-center rounded-full bg-primary text-primary-foreground"
                            aria-hidden
                          >
                            <Check className="h-4 w-4" />
                          </div>
                        ) : null}
                        <div className="min-w-0">
                          <p className="font-medium text-card-foreground">{svc.name}</p>
                          {svc.description ? (
                            <p className="text-sm text-muted-foreground">{svc.description}</p>
                          ) : null}
                          <p className="mt-1 text-xs text-muted-foreground">{svc.duration_min} мин</p>
                        </div>
                        {Number(svc.price) > 0 ? (
                          <span className="shrink-0 font-semibold text-primary">
                            {kopecksToRub(Number(svc.price))} ₽
                          </span>
                        ) : (
                          <span className="shrink-0 text-sm text-muted-foreground">Почасово</span>
                        )}
                      </CardContent>
                    </Card>
                  )})}
                </div>
              </div>
            ) : null}

            {showPublicTravelZone && master ? (
              <div className="mb-8">
                <h2 className="mb-2 text-xl font-semibold text-foreground">Зона выезда</h2>
                <p className="mb-4 text-sm text-muted-foreground">
                  На карте — ориентир по прямой: до {master.travel_radius_km} км от точки отсчёта.
                  Синяя область — куда мастер готов выезжать; красные круги — исключения внутри зоны.
                </p>
                <MasterTravelBaseMap
                  readOnly
                  apiKey={yandexMapsApiKey}
                  mapVersion={publicTravelMapVersion}
                  cityHint={master.city}
                  seedLat={master.travel_base_latitude ?? null}
                  seedLon={master.travel_base_longitude ?? null}
                  onPositionChange={noopTravelPosition}
                  travelRadiusKm={master.travel_radius_km}
                  excludeZones={publicExcludeZones}
                />
              </div>
            ) : null}

            <div>
              <h2 className="mb-6 text-xl font-semibold text-foreground">
                Отзывы {reviews.length > 0 && `(${reviews.length})`}
              </h2>
              <ReviewList
                targetId={master.id}
                targetType="master"
                reviews={reviews}
                onReviewAdded={reloadReviews}
              />
            </div>
          </div>

          <div className="lg:col-span-1">
            <Card className="sticky top-24 border-border">
              <CardHeader>
                <CardTitle className="text-xl text-card-foreground">Забронировать</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                {!user && (
                  <p className="text-sm text-muted-foreground">
                    <Link href="/auth/login" className="text-primary underline">
                      Войдите
                    </Link>
                    , чтобы оставить заявку
                  </p>
                )}

                <div>
                  <label className="mb-2 block text-sm font-medium text-card-foreground">Дата</label>
                  <Popover>
                    <PopoverTrigger asChild>
                      <Button variant="outline" className="w-full justify-start gap-2 text-left font-normal">
                        <CalendarIcon className="h-4 w-4 shrink-0" />
                        {date ? format(date, "d MMMM yyyy", { locale: ru }) : "Выберите дату"}
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent className="w-auto p-0" align="start">
                      <Calendar
                        mode="single"
                        selected={date}
                        onSelect={setDate}
                        initialFocus
                        disabled={(d) => isBefore(startOfDay(d), startOfDay(new Date()))}
                      />
                    </PopoverContent>
                  </Popover>
                </div>

                {master.services && master.services.length > 0 ? (
                  <div className="space-y-2">
                    <Label className="text-card-foreground">Услуги в заявке</Label>
                    {selectedServiceIds.length === 0 ? (
                      <p className="text-sm text-muted-foreground">
                        Почасовой расчёт. Выберите услуги на карточках слева.
                      </p>
                    ) : (
                      <ul className="max-h-40 space-y-1.5 overflow-y-auto rounded-md border border-border bg-muted/30 px-3 py-2 text-sm">
                        {selectedServiceIds.map((id) => {
                          const s = master.services?.find((x) => x.id === id);
                          if (!s) return null;
                          return (
                            <li key={id} className="flex justify-between gap-2">
                              <span className="line-clamp-2 text-card-foreground">{s.name}</span>
                              {Number(s.price) > 0 ? (
                                <span className="shrink-0 font-medium text-primary">
                                  {kopecksToRub(Number(s.price))} ₽
                                </span>
                              ) : (
                                <span className="shrink-0 text-muted-foreground">почасово</span>
                              )}
                            </li>
                          );
                        })}
                      </ul>
                    )}
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className="w-full"
                      disabled={selectedServiceIds.length === 0}
                      onClick={() => setSelectedServiceIds([])}
                    >
                      Только почасово
                    </Button>
                    <p className="text-xs text-muted-foreground">
                      Фиксированные услуги суммируются; не более одной услуги без цены в одной заявке.
                    </p>
                  </div>
                ) : null}

                <div className="space-y-2">
                  <Label className="text-card-foreground">Время начала</Label>
                  <Select value={time} onValueChange={setTime} disabled={!date}>
                    <SelectTrigger className="w-full font-normal">
                      <SelectValue
                        placeholder={!date ? "Сначала выберите дату" : "Выберите время"}
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {startTimeGrid.map((t) => (
                        <SelectItem key={t} value={t}>
                          {t}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                <div className="space-y-2">
                  <Label className="text-card-foreground">Окончание визита</Label>
                  <Select
                    value={timeTo}
                    onValueChange={setTimeTo}
                    disabled={!time || visitEndOptions.length === 0}
                  >
                    <SelectTrigger className="w-full font-normal">
                      <SelectValue
                        placeholder={
                          !time ? "Сначала выберите начало" : "Выберите время окончания"
                        }
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {visitEndOptions.map((t) => (
                        <SelectItem key={t} value={t}>
                          {t}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {Boolean(time && timeTo && !slotValid) && (
                    <p className="text-xs text-destructive">
                      Проверьте интервал: длительность от 30 мин до 12 ч, шаг 30 минут.
                    </p>
                  )}
                </div>

                <div>
                  <label className="mb-2 block text-sm font-medium text-card-foreground">Гости</label>
                  <div className="flex items-center gap-3">
                    <Button type="button" variant="outline" size="icon" disabled aria-disabled>
                      <Minus className="h-4 w-4" />
                    </Button>
                    <div className="flex items-center gap-2">
                      <Users className="h-4 w-4 text-muted-foreground" />
                      <span className="w-8 text-center font-medium">{guests}</span>
                    </div>
                    <Button type="button" variant="outline" size="icon" disabled aria-disabled>
                      <Plus className="h-4 w-4" />
                    </Button>
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    Заявка оформляется на одного клиента.
                  </p>
                </div>

                <div className="space-y-2">
                  <Label htmlFor="master-book-comment">Комментарий</Label>
                  <Textarea
                    id="master-book-comment"
                    value={comment}
                    onChange={(e) => setComment(e.target.value)}
                    rows={3}
                    placeholder="Пожелания по визиту (необязательно)"
                  />
                </div>

                <div className="border-t border-border pt-4">
                  {priceHint ? (
                    <div className="mb-4 flex items-center justify-between gap-2">
                      <span className="text-muted-foreground">К оплате</span>
                      <span className="text-2xl font-bold text-foreground">{priceHint}</span>
                    </div>
                  ) : (
                    <p className="mb-4 text-sm text-muted-foreground">
                      Стоимость появится после выбора услуги или длительности.
                    </p>
                  )}
                  <Button
                    className="w-full"
                    size="lg"
                    disabled={!user || !date || !time || !slotValid || bookMut.isPending}
                    onClick={onBook}
                  >
                    {bookMut.isPending ? "Отправка…" : token ? "Забронировать" : "Войти и забронировать"}
                  </Button>
                  {msg && (
                    <p
                      className={`mt-2 text-center text-sm ${
                        msg.startsWith("Заявка") ? "text-green-600" : "text-destructive"
                      }`}
                    >
                      {msg}
                    </p>
                  )}
                </div>
              </CardContent>
            </Card>
          </div>
        </div>
      </div>
      <Dialog open={photoGallery.lightboxOpen} onOpenChange={(open) => !open && photoGallery.closeLightbox()}>
        <DialogContent
          className="max-w-[96vw] border-0 bg-black/95 p-2 sm:p-4"
          showCloseButton={false}
          aria-describedby={undefined}
        >
          <DialogTitle className="sr-only">Фото мастера</DialogTitle>
          {photoGallery.current ? (
            <div className="relative h-[80vh] w-full">
              <Image
                src={photoGallery.current.src}
                alt=""
                fill
                className="object-contain"
                sizes="100vw"
                unoptimized
              />
              {photoGallery.count > 1 ? (
                <>
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    className="absolute left-2 top-1/2 z-10 h-10 w-10 -translate-y-1/2 border-white/40 bg-black/60 text-white hover:bg-black/70"
                    onClick={photoGallery.prev}
                    aria-label="Предыдущее фото"
                  >
                    <ChevronLeft className="h-5 w-5" />
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    className="absolute right-2 top-1/2 z-10 h-10 w-10 -translate-y-1/2 border-white/40 bg-black/60 text-white hover:bg-black/70"
                    onClick={photoGallery.next}
                    aria-label="Следующее фото"
                  >
                    <ChevronRight className="h-5 w-5" />
                  </Button>
                </>
              ) : null}
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
    </section>
  );
}
