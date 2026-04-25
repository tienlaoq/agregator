"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAuthStore } from "@/store/auth";
import Image from "next/image";
import {
  createMasterProfile,
  deleteMasterPhoto,
  getMyMasterProfile,
  patchMyMasterProfile,
  setMasterCoverPhoto,
  submitMasterForReview,
  uploadMasterPhoto,
  formatApiErrorMessage,
  venueMediaUrl,
} from "@/lib/api";
import type { MasterPhoto, MasterProfile, MasterServiceItem } from "@/lib/types";
import { MASTER_PROFILE_STATUS_LABELS } from "@/lib/types";
import { PhoneInput, getRawPhone, displayPhoneFromStored } from "@/components/banya/phone-input";
import { Plus, Trash2, ArrowLeft, Send, Upload, CircleAlert } from "lucide-react";

function kopecksToRub(k: number) {
  return k / 100;
}

function rubToKopecks(r: number) {
  return Math.round(r * 100);
}

/** Пустое поле вместо 0 — чтобы при вводе не получалось «05», «010». */
function numFieldValue(n: number, emptyWhenZero: boolean) {
  return emptyWhenZero && n === 0 ? "" : n;
}

function parseNonNegIntInput(raw: string, fallback: number) {
  if (raw === "") return fallback;
  const n = parseInt(raw, 10);
  return Number.isFinite(n) && n >= 0 ? n : fallback;
}

function parseNonNegFloatInput(raw: string, fallback: number) {
  if (raw === "") return fallback;
  const n = Number(raw);
  return Number.isFinite(n) && n >= 0 ? n : fallback;
}

function availabilityNoteFromStored(raw: string | undefined): string {
  if (!raw?.trim() || raw.trim() === "{}") return "";
  try {
    const o = JSON.parse(raw) as { note?: unknown };
    if (o && typeof o === "object" && typeof o.note === "string") return o.note;
  } catch {
    return "";
  }
  return "";
}

/** Сохраняем прочие ключи в JSON (если были), обновляем только человекочитаемую заметку. */
function mergeAvailabilityWithNote(prevRaw: string | undefined, note: string): string {
  let base: Record<string, unknown> = {};
  const raw = prevRaw?.trim() ?? "";
  if (raw && raw !== "{}") {
    try {
      const o = JSON.parse(raw);
      if (o && typeof o === "object" && !Array.isArray(o)) base = { ...(o as Record<string, unknown>) };
    } catch {
      base = {};
    }
  }
  const t = note.trim();
  if (t) base.note = t;
  else delete base.note;
  return JSON.stringify(base);
}

type ServiceLine = {
  key: string;
  id?: string;
  name: string;
  description: string;
  duration_min: number;
  price_rub: number;
};

const DEFAULT_SERVICE_DURATION_MIN = 60;

function newServiceLine(): ServiceLine {
  return {
    key: crypto.randomUUID(),
    name: "",
    description: "",
    duration_min: DEFAULT_SERVICE_DURATION_MIN,
    price_rub: 0,
  };
}

function servicesFromProfile(s: MasterServiceItem[]): ServiceLine[] {
  return s.map((x) => ({
    key: x.id || crypto.randomUUID(),
    id: x.id,
    name: x.name,
    description: x.description,
    duration_min: x.duration_min,
    price_rub: kopecksToRub(x.price),
  }));
}

const MAX_MASTER_PHOTOS = 12;

/** Как на master-service: 11 цифр, страна 7. */
function normalizeRussianMobileDigits(phone: string): string {
  const d = phone.replace(/\D/g, "");
  if (d.length === 11) {
    if (d.startsWith("8")) return "7" + d.slice(1);
    if (d.startsWith("7")) return d;
    return "";
  }
  if (d.length === 10) return "7" + d;
  return "";
}

function masterSubmitValidationMessage(body: Record<string, unknown>): string | null {
  const name = String(body.display_name ?? "").trim();
  if (!name) return "Укажите имя на платформе";
  const city = String(body.city ?? "").trim();
  if (!city) return "Укажите город";
  if (normalizeRussianMobileDigits(String(body.phone ?? "")) === "") {
    return "Укажите полный номер телефона в формате +7 (999) 123-45-67";
  }
  const bio = String(body.bio ?? "").trim();
  if (bio.length === 0 || [...bio].length < 20) {
    return "Описание «О себе» должно быть не короче 20 символов";
  }
  const svc = body.services;
  if (!Array.isArray(svc) || svc.length === 0) {
    return "Добавьте хотя бы одну услугу и укажите её название";
  }
  const plf = String(body.payout_legal_form ?? "").trim().toLowerCase();
  if (!["ip", "ooo", "individual", "self_employed"].includes(plf)) {
    return "Укажите форму получения выплат: ИП, ООО, физическое лицо или самозанятость";
  }
  return null;
}

function sortMasterPhotos(photos: MasterProfile["photos"]): MasterPhoto[] {
  if (!photos?.length) return [];
  return [...photos].sort(
    (a, b) => a.sort_order - b.sort_order || a.id.localeCompare(b.id),
  );
}

export default function MasterProfilePage() {
  const router = useRouter();
  const qc = useQueryClient();
  const { token, user, hydrated } = useAuthStore();
  const [createName, setCreateName] = useState("");
  const [error, setError] = useState("");
  const [submitWarning, setSubmitWarning] = useState("");
  const [photoError, setPhotoError] = useState("");

  const [displayName, setDisplayName] = useState("");
  const [bio, setBio] = useState("");
  const [phone, setPhone] = useState("");
  const [city, setCity] = useState("");
  const [payoutLegalForm, setPayoutLegalForm] = useState("");
  const [workFormat, setWorkFormat] = useState("both");
  const [travelRadius, setTravelRadius] = useState(0);
  const [experienceYears, setExperienceYears] = useState(0);
  const [hourlyRub, setHourlyRub] = useState(0);
  const [specText, setSpecText] = useState("");
  const [availabilityNote, setAvailabilityNote] = useState("");
  const [services, setServices] = useState<ServiceLine[]>([newServiceLine()]);

  useEffect(() => {
    if (hydrated && (!token || user?.role !== "master")) {
      router.push("/auth/login");
    }
  }, [hydrated, token, user, router]);

  const profileQuery = useQuery({
    queryKey: ["my-master-profile"],
    queryFn: getMyMasterProfile,
    enabled: !!token && user?.role === "master",
    retry: false,
  });

  const profile =
    profileQuery.data === null || profileQuery.data === undefined
      ? undefined
      : profileQuery.data;
  const notFound = profileQuery.isSuccess && profileQuery.data === null;

  useEffect(() => {
    if (!profile) return;
    setDisplayName(profile.display_name);
    setBio(profile.bio);
    setPhone(displayPhoneFromStored(profile.phone));
    setCity(profile.city);
    setPayoutLegalForm(profile.payout_legal_form ?? "");
    setWorkFormat(profile.work_format || "both");
    setTravelRadius(profile.travel_radius_km);
    setExperienceYears(profile.experience_years);
    setHourlyRub(kopecksToRub(profile.hourly_rate));
    setSpecText(profile.specializations?.join(", ") ?? "");
    setAvailabilityNote(availabilityNoteFromStored(profile.availability_json));
    setServices(
      profile.services?.length ? servicesFromProfile(profile.services) : [newServiceLine()],
    );
  }, [profile]);

  const createMut = useMutation({
    mutationFn: () => createMasterProfile(createName.trim()),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["my-master-profile"] });
      setError("");
    },
    onError: (e) => setError(formatApiErrorMessage(e, "Не удалось создать профиль")),
  });

  const buildMasterPatchBody = (): Record<string, unknown> => {
    const specs = specText
      .split(/[,;\n]+/)
      .map((x) => x.trim())
      .filter(Boolean);
    const svcPayload = services
      .filter((l) => l.name.trim())
      .map((l, i) => {
        const duration =
          l.duration_min >= 15 ? l.duration_min : DEFAULT_SERVICE_DURATION_MIN;
        const row: Record<string, unknown> = {
          name: l.name.trim(),
          description: l.description.trim(),
          duration_min: duration,
          price: rubToKopecks(l.price_rub),
          sort_order: i,
        };
        if (l.id) row.id = l.id;
        return row;
      });
    return {
      display_name: displayName.trim(),
      bio: bio.trim(),
      phone: getRawPhone(phone).trim(),
      city: city.trim(),
      payout_legal_form: payoutLegalForm.trim(),
      work_format: workFormat,
      travel_radius_km: travelRadius,
      experience_years: experienceYears,
      hourly_rate: rubToKopecks(hourlyRub),
      availability_json: mergeAvailabilityWithNote(profile?.availability_json, availabilityNote),
      specializations: specs,
      services: svcPayload,
    };
  };

  const saveMut = useMutation({
    mutationFn: () => patchMyMasterProfile(buildMasterPatchBody()),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["my-master-profile"] });
      setError("");
    },
    onError: (e) => setError(formatApiErrorMessage(e, "Не удалось сохранить")),
  });

  const submitMut = useMutation({
    mutationFn: () => submitMasterForReview(buildMasterPatchBody()),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["my-master-profile"] });
      setError("");
      setSubmitWarning("");
    },
    onError: (e) =>
      setError(formatApiErrorMessage(e, "Не удалось отправить на модерацию")),
  });

  const handleSubmitForReview = () => {
    const body = buildMasterPatchBody();
    const msg = masterSubmitValidationMessage(body);
    if (msg) {
      setSubmitWarning(msg);
      setError("");
      return;
    }
    setSubmitWarning("");
    submitMut.mutate();
  };

  useEffect(() => {
    if (!submitWarning) return;
    const msg = masterSubmitValidationMessage(buildMasterPatchBody());
    if (msg === null) setSubmitWarning("");
    else setSubmitWarning(msg);
  }, [
    submitWarning,
    displayName,
    bio,
    phone,
    city,
    payoutLegalForm,
    workFormat,
    travelRadius,
    experienceYears,
    hourlyRub,
    specText,
    availabilityNote,
    services,
    profile?.availability_json,
  ]);

  const uploadPhotoMu = useMutation({
    mutationFn: (file: File) => uploadMasterPhoto(file),
    onSuccess: () => {
      setPhotoError("");
      void qc.invalidateQueries({ queryKey: ["my-master-profile"] });
    },
    onError: (e: unknown) =>
      setPhotoError(
        formatApiErrorMessage(
          e,
          "Не удалось загрузить фото. Допустимы JPEG, PNG, WebP, до 5 МБ.",
        ),
      ),
  });

  const deletePhotoMu = useMutation({
    mutationFn: (photoId: string) => deleteMasterPhoto(photoId),
    onSuccess: () => {
      setPhotoError("");
      void qc.invalidateQueries({ queryKey: ["my-master-profile"] });
    },
    onError: (e: unknown) =>
      setPhotoError(formatApiErrorMessage(e, "Не удалось удалить фото.")),
  });

  const coverPhotoMu = useMutation({
    mutationFn: (photoId: string) => setMasterCoverPhoto(photoId),
    onSuccess: () => {
      setPhotoError("");
      void qc.invalidateQueries({ queryKey: ["my-master-profile"] });
    },
    onError: (e: unknown) =>
      setPhotoError(formatApiErrorMessage(e, "Не удалось назначить обложку.")),
  });

  if (!hydrated || !token || user?.role !== "master") return null;

  const meta = profile ? MASTER_PROFILE_STATUS_LABELS[profile.status] : null;
  const canSubmit =
    profile &&
    ["draft", "needs_revision", "rejected"].includes(profile.status);

  const sortedMasterPhotos = profile ? sortMasterPhotos(profile.photos) : [];
  const masterPhotoCount = sortedMasterPhotos.length;
  const canAddMasterPhotos = masterPhotoCount < MAX_MASTER_PHOTOS;

  return (
    <div className="container mx-auto max-w-2xl px-4 py-10">
      <Button variant="ghost" asChild className="mb-6 gap-2">
        <Link href="/owner/master">
          <ArrowLeft className="h-4 w-4" />
          Назад в кабинет
        </Link>
      </Button>

      <h1 className="mb-2 text-2xl font-bold">Профиль мастера</h1>
      <p className="mb-6 text-muted-foreground">
        До одобрения модератором профиль не показывается клиентам в каталоге.
      </p>

      {error && <p className="mb-4 text-sm text-destructive">{error}</p>}

      {profileQuery.isLoading && <p className="text-muted-foreground">Загрузка...</p>}

      {notFound && (
        <Card>
          <CardHeader>
            <CardTitle>Создать профиль</CardTitle>
            <CardDescription>Как к вам обращаться на платформе</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="createName">Отображаемое имя</Label>
              <Input
                id="createName"
                value={createName}
                onChange={(e) => setCreateName(e.target.value)}
                placeholder="Например: Пармастер Алексей"
              />
            </div>
            <Button
              disabled={!createName.trim() || createMut.isPending}
              onClick={() => createMut.mutate()}
            >
              {createMut.isPending ? "Создание..." : "Создать профиль"}
            </Button>
          </CardContent>
        </Card>
      )}

      {!profileQuery.isLoading && profile && (
        <>
          <div className="mb-6 flex flex-wrap items-center gap-2">
            <span className="text-sm text-muted-foreground">Статус:</span>
            <Badge>{meta?.label ?? profile.status}</Badge>
          </div>

          {profile.moderation_comment && (
            <Card className="mb-6 border-amber-200 bg-amber-50/50">
              <CardHeader className="py-3">
                <CardTitle className="text-base">Комментарий модератора</CardTitle>
                <CardDescription className="text-foreground whitespace-pre-wrap">
                  {profile.moderation_comment}
                </CardDescription>
              </CardHeader>
            </Card>
          )}

          <Card className="mb-6">
            <CardHeader className="pb-4">
              <CardTitle className="text-lg">Фотографии</CardTitle>
              <CardDescription>
                Фото для карточки в каталоге. JPEG, PNG или WebP, до 5 МБ каждое,
                не более {MAX_MASTER_PHOTOS}. Первое загруженное станет обложкой;
                обложку можно сменить.
              </CardDescription>
            </CardHeader>
            <CardContent>
              {photoError ? (
                <p className="mb-3 text-sm text-destructive" role="alert">
                  {photoError}
                </p>
              ) : null}
              <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4">
                {sortedMasterPhotos.map((p) => (
                  <div
                    key={p.id}
                    className="group relative aspect-square overflow-hidden rounded-lg border border-border"
                  >
                    <Image
                      src={venueMediaUrl(p.url)}
                      alt=""
                      fill
                      className="object-cover"
                      sizes="(max-width: 768px) 50vw, 25vw"
                      unoptimized
                    />
                    {p.is_cover ? (
                      <div className="absolute left-2 top-2">
                        <Badge variant="secondary" className="text-xs">
                          Обложка
                        </Badge>
                      </div>
                    ) : null}
                    <div className="absolute inset-x-0 bottom-0 flex flex-wrap justify-center gap-1 bg-black/55 p-2 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100">
                      {!p.is_cover ? (
                        <Button
                          type="button"
                          size="sm"
                          variant="secondary"
                          className="h-8 text-xs"
                          disabled={coverPhotoMu.isPending}
                          onClick={() => coverPhotoMu.mutate(p.id)}
                        >
                          В обложку
                        </Button>
                      ) : null}
                      <Button
                        type="button"
                        size="sm"
                        variant="destructive"
                        className="h-8 px-2"
                        disabled={deletePhotoMu.isPending}
                        onClick={() => {
                          if (
                            window.confirm(
                              "Удалить это фото? Его нельзя будет восстановить.",
                            )
                          ) {
                            deletePhotoMu.mutate(p.id);
                          }
                        }}
                        aria-label="Удалить фото"
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                ))}
                <label
                  className={
                    canAddMasterPhotos && !uploadPhotoMu.isPending
                      ? "flex aspect-square cursor-pointer flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed border-border bg-muted/30 text-muted-foreground transition-colors hover:border-primary hover:bg-muted/50 hover:text-primary"
                      : "flex aspect-square cursor-not-allowed flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed border-border bg-muted/20 text-muted-foreground opacity-60"
                  }
                >
                  <Upload className="h-8 w-8" />
                  <span className="px-2 text-center text-xs">
                    {uploadPhotoMu.isPending
                      ? "Загрузка…"
                      : canAddMasterPhotos
                        ? "Загрузить"
                        : "Лимит фото"}
                  </span>
                  <input
                    type="file"
                    accept="image/jpeg,image/png,image/webp"
                    className="hidden"
                    multiple
                    disabled={!canAddMasterPhotos || uploadPhotoMu.isPending}
                    onChange={async (e) => {
                      const { files } = e.target;
                      if (!files?.length) return;
                      for (const f of Array.from(files)) {
                        try {
                          await uploadPhotoMu.mutateAsync(f);
                        } catch {
                          break;
                        }
                      }
                      e.target.value = "";
                    }}
                  />
                </label>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardContent className="space-y-4 pt-6">
              <div className="space-y-2">
                <Label>Имя на платформе</Label>
                <Input value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
              </div>
              <div className="space-y-2">
                <Label>О себе</Label>
                <Textarea rows={4} value={bio} onChange={(e) => setBio(e.target.value)} />
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="masterPhone">Телефон</Label>
                  <PhoneInput
                    id="masterPhone"
                    value={phone}
                    onChange={setPhone}
                    placeholder="+7 (999) 123-45-67"
                  />
                </div>
                <div className="space-y-2">
                  <Label>Город</Label>
                  <Input value={city} onChange={(e) => setCity(e.target.value)} />
                </div>
              </div>
              <div className="space-y-2">
                <Label>Форма получения выплат</Label>
                <p className="text-xs text-muted-foreground">
                  Оплата услуг проходит через платформу; выплаты вам переводятся с учётом вашего
                  официального статуса: ИП, ООО, физическое лицо или самозанятость.
                </p>
                <Select
                  value={payoutLegalForm || "__none"}
                  onValueChange={(v) => setPayoutLegalForm(v === "__none" ? "" : v)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Выберите вариант" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__none">Не выбрано</SelectItem>
                    <SelectItem value="ip">Индивидуальный предприниматель (ИП)</SelectItem>
                    <SelectItem value="ooo">Общество с ограниченной ответственностью (ООО)</SelectItem>
                    <SelectItem value="individual">Физическое лицо</SelectItem>
                    <SelectItem value="self_employed">Самозанятость</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>Формат работы</Label>
                <Select value={workFormat} onValueChange={setWorkFormat}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="venue">В бане / у заведения</SelectItem>
                    <SelectItem value="mobile">Выезд к клиенту</SelectItem>
                    <SelectItem value="both">И то и другое</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label>Радиус выезда (км)</Label>
                  <Input
                    type="number"
                    min={0}
                    inputMode="numeric"
                    value={numFieldValue(travelRadius, true)}
                    onChange={(e) =>
                      setTravelRadius(parseNonNegIntInput(e.target.value, 0))
                    }
                  />
                </div>
                <div className="space-y-2">
                  <Label>Опыт (лет)</Label>
                  <Input
                    type="number"
                    min={0}
                    inputMode="numeric"
                    value={numFieldValue(experienceYears, true)}
                    onChange={(e) =>
                      setExperienceYears(parseNonNegIntInput(e.target.value, 0))
                    }
                  />
                </div>
              </div>
              <div className="space-y-2">
                <Label>Базовая ставка (₽/час)</Label>
                <p className="text-xs text-muted-foreground">
                  В каталоге в строке «от … ₽ / час» показывается минимальная цена среди ваших услуг; если
                  услуг с ценой нет — используется эта ставка.
                </p>
                <Input
                  type="number"
                  min={0}
                  step={100}
                  inputMode="decimal"
                  value={numFieldValue(hourlyRub, true)}
                  onChange={(e) =>
                    setHourlyRub(parseNonNegFloatInput(e.target.value, 0))
                  }
                />
              </div>
              <div className="space-y-2">
                <Label>Специализации (через запятую)</Label>
                <Input
                  value={specText}
                  onChange={(e) => setSpecText(e.target.value)}
                  placeholder="парения, массаж, ароматерапия"
                />
              </div>
              <div className="space-y-2">
                <Label>Когда вы на связи или готовы принимать заказы</Label>
                <p className="text-xs text-muted-foreground">
                  Необязательно. Клиенты увидят это в карточке мастера.
                </p>
                <Textarea
                  rows={3}
                  value={availabilityNote}
                  onChange={(e) => setAvailabilityNote(e.target.value)}
                  placeholder="Например: будни с 10:00 до 20:00, выходные — по договорённости"
                />
              </div>

              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <Label>Услуги</Label>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="gap-1"
                    onClick={() => setServices((s) => [...s, newServiceLine()])}
                  >
                    <Plus className="h-4 w-4" />
                    Добавить
                  </Button>
                </div>
                {services.map((line, idx) => (
                  <div key={line.key} className="rounded-lg border border-border p-3 space-y-2">
                    <div className="flex justify-end">
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-destructive"
                        onClick={() =>
                          setServices((s) => s.filter((_, i) => i !== idx))
                        }
                        disabled={services.length <= 1}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                    <Input
                      placeholder="Название услуги"
                      value={line.name}
                      onChange={(e) =>
                        setServices((s) =>
                          s.map((x, i) =>
                            i === idx ? { ...x, name: e.target.value } : x,
                          ),
                        )
                      }
                    />
                    <Textarea
                      placeholder="Описание"
                      rows={2}
                      value={line.description}
                      onChange={(e) =>
                        setServices((s) =>
                          s.map((x, i) =>
                            i === idx ? { ...x, description: e.target.value } : x,
                          ),
                        )
                      }
                    />
                    <div className="grid grid-cols-2 gap-2">
                      <div>
                        <Label className="text-xs">Минут</Label>
                        <Input
                          type="number"
                          min={15}
                          inputMode="numeric"
                          value={numFieldValue(line.duration_min, true)}
                          onChange={(e) =>
                            setServices((s) =>
                              s.map((x, i) =>
                                i === idx
                                  ? {
                                      ...x,
                                      duration_min: parseNonNegIntInput(
                                        e.target.value,
                                        0,
                                      ),
                                    }
                                  : x,
                              ),
                            )
                          }
                        />
                      </div>
                      <div>
                        <Label className="text-xs">Цена ₽</Label>
                        <Input
                          type="number"
                          min={0}
                          inputMode="numeric"
                          value={numFieldValue(line.price_rub, true)}
                          onChange={(e) =>
                            setServices((s) =>
                              s.map((x, i) =>
                                i === idx
                                  ? {
                                      ...x,
                                      price_rub: parseNonNegFloatInput(
                                        e.target.value,
                                        0,
                                      ),
                                    }
                                  : x,
                              ),
                            )
                          }
                        />
                      </div>
                    </div>
                  </div>
                ))}
              </div>

              <div className="flex flex-col gap-3 pt-4 sm:flex-row">
                <Button
                  className="flex-1"
                  disabled={saveMut.isPending}
                  onClick={() => saveMut.mutate()}
                >
                  {saveMut.isPending ? "Сохранение..." : "Сохранить изменения"}
                </Button>
                {canSubmit && (
                  <div className="flex min-w-0 flex-1 flex-col items-stretch gap-1 sm:max-w-md">
                    <Button
                      variant="secondary"
                      className="w-full shrink-0 gap-2 sm:w-auto sm:self-start"
                      disabled={submitMut.isPending || saveMut.isPending}
                      onClick={handleSubmitForReview}
                    >
                      <Send className="h-4 w-4" />
                      {submitMut.isPending
                        ? "Сохранение и отправка..."
                        : "Отправить на модерацию"}
                    </Button>
                    {submitWarning ? (
                      <p
                        role="alert"
                        className="text-pretty pl-0.5 text-sm leading-snug text-muted-foreground"
                      >
                        <CircleAlert
                          className="mr-1.5 inline-block size-3.5 shrink-0 align-[-0.15em] text-amber-600/85 dark:text-amber-500/85"
                          aria-hidden
                        />
                        {submitWarning}
                      </p>
                    ) : null}
                  </div>
                )}
              </div>
              {profile.status === "pending_review" && (
                <p className="text-sm text-muted-foreground">
                  Профиль на проверке. После решения модератора статус обновится здесь.
                </p>
              )}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}
