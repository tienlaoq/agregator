"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import Image from "next/image";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  type QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import {
  deleteVenueHallPhoto,
  deleteVenuePhoto,
  formatApiErrorMessage,
  getOwnerVenues,
  setVenueCoverPhoto,
  setVenueHallCoverPhoto,
  submitVenueForReview,
  updateVenue,
  uploadVenueHallPhoto,
  uploadVenuePhoto,
  venueMediaUrl,
} from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import {
  VENUE_CARD_STATUS_LABELS,
  buildVenueSocialLinksPayload,
  parseVenueSocialLinks,
  newVenueHallLine,
  type UpdateVenueRequest,
  type VenueUpdatePayload,
  trimVenueSocialLinks,
  venueServiceLinesForApi,
  venueServicesToFormLines,
  type Venue,
  type VenueHallFormLine,
  type VenuePhoto,
} from "@/lib/types";
import { PartnerVenueRegistrationCard } from "@/components/banya/partner-venue-registration-card";
import { OwnerSlotBlocksSection } from "@/components/banya/owner-slot-blocks-section";
import { VenueServicesFields } from "@/components/banya/venue-services-fields";
import { VenueSocialLinksFields } from "@/components/banya/venue-social-links-fields";
import { getRawPhone } from "@/components/banya/phone-input";
import {
  applyPartnerVenuePatchToUpdateForm,
  partnerVenueValuesFromUpdateForm,
  type PartnerVenueFormValues,
} from "@/lib/partner-venue";
import {
  AlertTriangle,
  ArrowLeft,
  CheckCircle2,
  Circle,
  Clock,
  FileEdit,
  Info,
  Plus,
  Trash2,
  Upload,
  X,
  XCircle,
} from "lucide-react";

const uuidRe =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

type VenueStatusUI =
  | "draft"
  | "pending_review"
  | "active"
  | "rejected"
  | "suspended";

const statusConfig: Record<
  VenueStatusUI,
  {
    label: string;
    icon: typeof CheckCircle2;
    className: string;
    bgClass: string;
  }
> = {
  draft: {
    label: "Черновик",
    icon: FileEdit,
    className:
      "border-slate-200 bg-slate-50 text-slate-800 hover:bg-slate-50 dark:border-slate-600 dark:bg-slate-900/40 dark:text-slate-100",
    bgClass:
      "border-slate-200 bg-slate-50 dark:border-slate-600 dark:bg-slate-900/25",
  },
  active: {
    label: "Активно",
    icon: CheckCircle2,
    className:
      "border-green-200 bg-green-50 text-green-800 hover:bg-green-50 dark:border-green-900 dark:bg-green-950/40 dark:text-green-100",
    bgClass: "border-green-200 bg-green-50 dark:border-green-900 dark:bg-green-950/20",
  },
  pending_review: {
    label: "На модерации",
    icon: Clock,
    className:
      "border-amber-200 bg-amber-50 text-amber-900 hover:bg-amber-50 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-100",
    bgClass: "border-amber-200 bg-amber-50 dark:border-amber-900 dark:bg-amber-950/20",
  },
  rejected: {
    label: "Отклонено",
    icon: XCircle,
    className:
      "border-red-200 bg-red-50 text-red-800 hover:bg-red-50 dark:border-red-900 dark:bg-red-950/40 dark:text-red-100",
    bgClass: "border-red-200 bg-red-50 dark:border-red-900 dark:bg-red-950/20",
  },
  suspended: {
    label: "Приостановлено",
    icon: AlertTriangle,
    className:
      "border-stone-200 bg-stone-100 text-stone-800 hover:bg-stone-100 dark:border-stone-700 dark:bg-stone-900/50 dark:text-stone-100",
    bgClass: "border-stone-200 bg-stone-100 dark:border-stone-700 dark:bg-stone-900/30",
  },
};

const PRESET_AMENITIES = [
  { id: "pool", label: "Бассейн" },
  { id: "parking", label: "Парковка" },
  { id: "wifi", label: "Wi-Fi" },
  { id: "veniks", label: "Веники" },
  { id: "rest_room", label: "Комната отдыха" },
  { id: "bar", label: "Бар" },
  { id: "shower", label: "Душевая" },
  { id: "jacuzzi", label: "Джакузи" },
  { id: "massage", label: "Массаж" },
  { id: "karaoke", label: "Караоке" },
  { id: "billiards", label: "Бильярд" },
  { id: "kitchen", label: "Кухня" },
] as const;

const PRESET_LABEL_SET = new Set(
  PRESET_AMENITIES.map((a) => a.label as string),
);

type WorkingHoursDraft = {
  weekdays: { from: string; to: string };
  weekends: { from: string; to: string };
};

/** Пустой черновик: время не подставляем — партнёр заполняет сам. */
const EMPTY_WORKING_HOURS: WorkingHoursDraft = {
  weekdays: { from: "", to: "" },
  weekends: { from: "", to: "" },
};

function getWorkingHoursDraft(raw: string): WorkingHoursDraft {
  try {
    const o = JSON.parse(raw) as unknown;
    if (
      o &&
      typeof o === "object" &&
      "weekdays" in o &&
      "weekends" in o &&
      o.weekdays &&
      typeof o.weekdays === "object" &&
      o.weekends &&
      typeof o.weekends === "object"
    ) {
      const w = o.weekdays as Record<string, unknown>;
      const e = o.weekends as Record<string, unknown>;
      if (
        typeof w.from === "string" &&
        typeof w.to === "string" &&
        typeof e.from === "string" &&
        typeof e.to === "string"
      ) {
        return {
          weekdays: { from: w.from, to: w.to },
          weekends: { from: e.from, to: e.to },
        };
      }
    }
  } catch {
    /* empty */
  }
  return {
    weekdays: { ...EMPTY_WORKING_HOURS.weekdays },
    weekends: { ...EMPTY_WORKING_HOURS.weekends },
  };
}

/** Соответствует venue-service validateVenueVerificationCore (кнопка «На проверку»). */
function draftVerificationOkForSubmit(
  form: Pick<
    UpdateVenueRequest,
    "legal_entity_name" | "inn" | "ogrn" | "public_listing_url"
  >,
): boolean {
  const legal = form.legal_entity_name.trim();
  const legalLen = [...legal].length;
  if (legalLen < 3 || legalLen > 500) return false;
  const inn = form.inn.replace(/\D/g, "");
  if (inn.length !== 10 && inn.length !== 12) return false;
  const ogrn = form.ogrn.replace(/\D/g, "");
  if (inn.length === 10 && ogrn.length !== 13) return false;
  if (inn.length === 12 && ogrn.length !== 15) return false;
  const raw = form.public_listing_url.trim();
  try {
    const u = new URL(raw);
    if (u.protocol !== "http:" && u.protocol !== "https:") return false;
    const host = u.hostname.toLowerCase();
    if (!host || host.includes("localhost") || host.startsWith("127.")) {
      return false;
    }
  } catch {
    return false;
  }
  return true;
}

function venueHallsToForm(v: Venue): VenueHallFormLine[] {
  const list = v.halls && v.halls.length > 0 ? v.halls : [];
  if (list.length === 0) return [newVenueHallLine()];
  return list.map((h) => ({
    clientKey: h.id,
    id: h.id,
    name: h.name,
    price_from: h.price_from,
    capacity: h.capacity ?? 8,
    amenities: [...(h.amenities ?? [])],
    photos: [...(h.photos ?? [])],
  }));
}

function venueToForm(v: Venue): UpdateVenueRequest {
  return {
    name: v.name,
    description: v.description,
    address: v.address,
    city: v.city,
    phone: v.phone ?? "",
    latitude:
      v.latitude != null && !Number.isNaN(v.latitude) ? v.latitude : 55.7558,
    longitude:
      v.longitude != null && !Number.isNaN(v.longitude) ? v.longitude : 37.6173,
    working_hours: v.working_hours?.trim() || "{}",
    halls: venueHallsToForm(v),
    services: venueServicesToFormLines(v.services ?? []),
    legal_entity_name: v.legal_entity_name ?? "",
    inn: v.inn ?? "",
    ogrn: v.ogrn ?? "",
    public_listing_url: v.public_listing_url ?? "",
    verification_note: v.verification_note?.trim() || undefined,
    social_links: parseVenueSocialLinks(v.social_links),
  };
}

function serializeForm(f: UpdateVenueRequest): string {
  return JSON.stringify({
    ...f,
    halls: f.halls.map((h) => ({
      k: h.clientKey,
      id: h.id ?? "",
      name: h.name,
      price_from: h.price_from,
      capacity: h.capacity,
      amenities: [...h.amenities].sort(),
    })),
    verification_note: f.verification_note ?? "",
    social: JSON.stringify(buildVenueSocialLinksPayload(f.social_links)),
    services: (f.services ?? []).map((s) => ({
      name: s.name.trim(),
      price: s.price,
      duration_min: s.duration_min,
      description: s.description.trim(),
    })),
  });
}

function hallsPayloadForApi(form: UpdateVenueRequest) {
  return form.halls
    .map((h, i) => ({
      id: h.id,
      name: h.name.trim(),
      price_from: h.price_from,
      capacity: h.capacity,
      amenities: [...h.amenities],
      sort_order: i,
    }))
    .filter((h) => h.name !== "");
}

/** Тело PATCH (как при «Сохранить»), без побочных эффектов навигации. */
function buildVenueUpdatePayload(form: UpdateVenueRequest): VenueUpdatePayload {
  return {
    ...form,
    phone: getRawPhone(form.phone),
    inn: form.inn.replace(/\D/g, ""),
    ogrn: form.ogrn.replace(/\D/g, ""),
    verification_note: form.verification_note?.trim() || undefined,
    social_links: trimVenueSocialLinks(form.social_links),
    services: venueServiceLinesForApi(form.services),
    halls: hallsPayloadForApi(form),
  };
}

/** Подставить id и фото залов из ответа сервера в текущий черновик формы. */
function mergeHallIdsFromUpdatedVenue(
  prev: UpdateVenueRequest,
  updated: Venue,
): UpdateVenueRequest {
  const srvHalls = updated.halls ?? [];
  const nextHalls = prev.halls.map((line, idx) => {
    if (line.id) return line;
    const name = line.name.trim();
    if (!name) return line;
    const byIndex = srvHalls[idx];
    const srv =
      byIndex?.name.trim() === name
        ? byIndex
        : srvHalls.find((h) => h.name.trim() === name);
    if (!srv?.id) return line;
    return {
      ...line,
      id: srv.id,
      clientKey: srv.id,
      photos: [...(srv.photos ?? line.photos ?? [])],
    };
  });
  return { ...prev, halls: nextHalls };
}

function sortPhotos(photos?: VenuePhoto[]): VenuePhoto[] {
  if (!photos?.length) return [];
  return [...photos].sort(
    (a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0),
  );
}

/** Merge venue from mutation response so UI updates before refetch (and refetch stays authoritative). */
function patchOwnerVenueInCache(qc: QueryClient, updated: Venue) {
  qc.setQueryData<Venue[]>(["owner-venues"], (prev) => {
    if (!prev?.length) return prev;
    let found = false;
    const next = prev.map((v) => {
      if (v.id !== updated.id) return v;
      found = true;
      return { ...v, ...updated };
    });
    return found ? next : prev;
  });
}

export default function EditOwnerVenuePage() {
  const params = useParams<{ venueId: string }>();
  const venueId = params.venueId;
  const router = useRouter();
  const queryClient = useQueryClient();
  const { token, user, hydrated } = useAuthStore();
  const [form, setForm] = useState<UpdateVenueRequest | null>(null);
  const [hallAmenityDraft, setHallAmenityDraft] = useState<Record<string, string>>(
    {},
  );
  const [error, setError] = useState("");
  const [photoError, setPhotoError] = useState("");
  const submittingRef = useRef(false);
  const baselineSerializedRef = useRef("");

  const validId = typeof venueId === "string" && uuidRe.test(venueId);

  const { data: venues, isLoading, isFetching } = useQuery({
    queryKey: ["owner-venues"],
    queryFn: getOwnerVenues,
    enabled: !!token && user?.role === "venue_owner" && validId,
    // Всегда тянем свежий список при заходе на страницу карточки: глобальный
    // staleTime (60с) мог отдать устаревший кэш, в котором только что созданного
    // заведения ещё нет — и страница ложно показывала «Заведение не найдено».
    staleTime: 0,
    refetchOnMount: "always",
  });

  const venue = useMemo(
    () => venues?.find((v) => v.id === venueId) ?? null,
    [venues, venueId],
  );

  useEffect(() => {
    if (hydrated && (!token || user?.role !== "venue_owner")) {
      router.push("/auth/login");
    }
  }, [hydrated, token, user, router]);

  useEffect(() => {
    setForm(null);
    baselineSerializedRef.current = "";
  }, [venueId]);

  useEffect(() => {
    if (!venue) return;
    setForm((prev) => {
      if (prev) return prev;
      const f = venueToForm(venue);
      baselineSerializedRef.current = serializeForm(f);
      return f;
    });
  }, [venue]);

  /** Подставляем id/фото залов с сервера в уже открытую форму (кэш мог обновиться позже первого init). */
  useEffect(() => {
    const srvHalls = venue?.halls;
    if (!srvHalls?.length) return;
    setForm((prev) => {
      if (!prev) return prev;
      let changed = false;
      const nextHalls = prev.halls.map((line, idx) => {
        if (line.id) return line;
        const name = line.name.trim();
        if (!name) return line;
        const byIndex = srvHalls[idx];
        const srv =
          byIndex?.name.trim() === name
            ? byIndex
            : srvHalls.find((h) => h.name.trim() === name);
        if (!srv?.id) return line;
        changed = true;
        return {
          ...line,
          id: srv.id,
          clientKey: srv.id,
          photos: [...(srv.photos ?? line.photos ?? [])],
        };
      });
      if (!changed) return prev;
      return { ...prev, halls: nextHalls };
    });
  }, [venue]);

  const updateField = <K extends keyof UpdateVenueRequest>(
    key: K,
    value: UpdateVenueRequest[K],
  ) => {
    setForm((prev) => (prev ? { ...prev, [key]: value } : prev));
  };

  const workingDraft = form
    ? getWorkingHoursDraft(form.working_hours)
    : EMPTY_WORKING_HOURS;

  const setWorkingDraft = (next: WorkingHoursDraft) => {
    updateField("working_hours", JSON.stringify(next));
  };

  const updateHall = (index: number, patch: Partial<VenueHallFormLine>) => {
    if (!form) return;
    const next = [...form.halls];
    next[index] = { ...next[index], ...patch };
    updateField("halls", next);
  };

  const toggleHallPresetAmenity = (hallIndex: number, label: string) => {
    if (!form) return;
    const h = form.halls[hallIndex];
    if (!h) return;
    const has = h.amenities.includes(label);
    updateHall(hallIndex, {
      amenities: has
        ? h.amenities.filter((x) => x !== label)
        : [...h.amenities, label],
    });
  };

  const removeHallCustomAmenity = (hallIndex: number, label: string) => {
    if (!form) return;
    const h = form.halls[hallIndex];
    if (!h) return;
    updateHall(hallIndex, {
      amenities: h.amenities.filter((x) => x !== label),
    });
  };

  const addHallCustomAmenity = (hallIndex: number) => {
    if (!form) return;
    const h = form.halls[hallIndex];
    if (!h) return;
    const t = (hallAmenityDraft[h.clientKey] ?? "").trim();
    if (!t || h.amenities.includes(t)) return;
    updateHall(hallIndex, { amenities: [...h.amenities, t] });
    setHallAmenityDraft((d) => ({ ...d, [h.clientKey]: "" }));
  };

  const addHall = () => {
    if (!form) return;
    updateField("halls", [...form.halls, newVenueHallLine()]);
  };

  const removeHall = (index: number) => {
    if (!form) return;
    if (form.halls.length <= 1) return;
    updateField(
      "halls",
      form.halls.filter((_, i) => i !== index),
    );
  };

  const mutation = useMutation({
    mutationFn: (payload: VenueUpdatePayload) =>
      updateVenue(venueId as string, payload),
    onSuccess: (updated) => {
      patchOwnerVenueInCache(queryClient, updated);
      void queryClient.invalidateQueries({ queryKey: ["owner-venues"] });
      router.push("/owner/venues");
    },
    onError: () => {
      setError("Не удалось сохранить. Проверьте соединение и попробуйте снова.");
    },
  });

  /** Тихое сохранение карточки (остаёмся на странице) — чтобы у нового зала появился id и можно было загрузить фото. */
  const persistVenueMutation = useMutation({
    mutationFn: (payload: VenueUpdatePayload) =>
      updateVenue(venueId as string, payload),
    onSuccess: (updated) => {
      patchOwnerVenueInCache(queryClient, updated);
      void queryClient.invalidateQueries({ queryKey: ["owner-venues"] });
    },
    onError: (e: unknown) => {
      setPhotoError(
        formatApiErrorMessage(
          e,
          "Не удалось сохранить карточку для загрузки фото зала.",
        ),
      );
    },
  });

  const uploadPhotoMu = useMutation({
    mutationFn: (file: File) => uploadVenuePhoto(venueId as string, file),
    onSuccess: (updated) => {
      setPhotoError("");
      patchOwnerVenueInCache(queryClient, updated);
      void queryClient.invalidateQueries({ queryKey: ["owner-venues"] });
    },
    onError: (e: unknown) => {
      setPhotoError(
        formatApiErrorMessage(
          e,
          "Не удалось загрузить фото. Допустимы JPEG, PNG, WebP, до 5 МБ, не более 24 фото.",
        ),
      );
    },
  });

  const deletePhotoMu = useMutation({
    mutationFn: (photoId: string) =>
      deleteVenuePhoto(venueId as string, photoId),
    onSuccess: (updated) => {
      setPhotoError("");
      patchOwnerVenueInCache(queryClient, updated);
      void queryClient.invalidateQueries({ queryKey: ["owner-venues"] });
    },
    onError: (e: unknown) =>
      setPhotoError(
        formatApiErrorMessage(e, "Не удалось удалить фото."),
      ),
  });

  const coverPhotoMu = useMutation({
    mutationFn: (photoId: string) =>
      setVenueCoverPhoto(venueId as string, photoId),
    onSuccess: (updated) => {
      setPhotoError("");
      patchOwnerVenueInCache(queryClient, updated);
      void queryClient.invalidateQueries({ queryKey: ["owner-venues"] });
    },
    onError: (e: unknown) =>
      setPhotoError(
        formatApiErrorMessage(e, "Не удалось назначить обложку."),
      ),
  });

  const mergeHallsPhotosFromVenue = (updated: Venue) => {
    setForm((prev) => {
      if (!prev) return prev;
      return {
        ...prev,
        halls: prev.halls.map((line) => {
          const srv = updated.halls?.find((x) => x.id === line.id);
          return srv
            ? { ...line, photos: [...(srv.photos ?? [])] }
            : line;
        }),
      };
    });
  };

  const uploadHallPhotoMu = useMutation({
    mutationFn: ({ hallId, file }: { hallId: string; file: File }) =>
      uploadVenueHallPhoto(venueId as string, hallId, file),
    onSuccess: (updated) => {
      setPhotoError("");
      patchOwnerVenueInCache(queryClient, updated);
      void queryClient.invalidateQueries({ queryKey: ["owner-venues"] });
      mergeHallsPhotosFromVenue(updated);
    },
    onError: (e: unknown) => {
      setPhotoError(
        formatApiErrorMessage(
          e,
          "Не удалось загрузить фото зала (до 16 фото на зал, до 5 МБ).",
        ),
      );
    },
  });

  const deleteHallPhotoMu = useMutation({
    mutationFn: ({
      hallId,
      photoId,
    }: {
      hallId: string;
      photoId: string;
    }) => deleteVenueHallPhoto(venueId as string, hallId, photoId),
    onSuccess: (updated) => {
      setPhotoError("");
      patchOwnerVenueInCache(queryClient, updated);
      void queryClient.invalidateQueries({ queryKey: ["owner-venues"] });
      mergeHallsPhotosFromVenue(updated);
    },
    onError: (e: unknown) =>
      setPhotoError(formatApiErrorMessage(e, "Не удалось удалить фото зала.")),
  });

  const coverHallPhotoMu = useMutation({
    mutationFn: ({
      hallId,
      photoId,
    }: {
      hallId: string;
      photoId: string;
    }) => setVenueHallCoverPhoto(venueId as string, hallId, photoId),
    onSuccess: (updated) => {
      setPhotoError("");
      patchOwnerVenueInCache(queryClient, updated);
      void queryClient.invalidateQueries({ queryKey: ["owner-venues"] });
      mergeHallsPhotosFromVenue(updated);
    },
    onError: (e: unknown) =>
      setPhotoError(
        formatApiErrorMessage(e, "Не удалось назначить обложку зала."),
      ),
  });

  const [submitReviewError, setSubmitReviewError] = useState("");

  const submitReviewMu = useMutation({
    mutationFn: () => submitVenueForReview(venueId as string),
    onSuccess: (updated) => {
      setSubmitReviewError("");
      patchOwnerVenueInCache(queryClient, updated);
      void queryClient.invalidateQueries({ queryKey: ["owner-venues"] });
      const nextForm = venueToForm(updated);
      setForm(nextForm);
      baselineSerializedRef.current = serializeForm(nextForm);
    },
    onError: (e: unknown) => {
      setSubmitReviewError(
        formatApiErrorMessage(
          e,
          "Не удалось отправить на проверку. Сохраните изменения и попробуйте снова.",
        ),
      );
    },
  });

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!form || submittingRef.current || !validId) return;
    if (!form.halls.some((h) => h.name.trim())) {
      setError("Укажите название хотя бы для одного зала.");
      return;
    }
    submittingRef.current = true;
    setError("");
    try {
      await mutation.mutateAsync(buildVenueUpdatePayload(form));
    } finally {
      submittingRef.current = false;
    }
  };

  const handleCancel = () => {
    router.push("/owner/venues");
  };

  if (!hydrated || !token) return null;

  if (!validId) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-10">
        <p className="text-sm text-muted-foreground">Некорректная ссылка.</p>
        <Button asChild variant="link" className="mt-2 h-auto px-0 text-sm">
          <Link href="/owner/venues">← Мои заведения</Link>
        </Button>
      </div>
    );
  }

  // Показываем скелетон пока:
  //  - идёт первичная загрузка списка, либо
  //  - список ещё не пришёл, либо
  //  - список пришёл (возможно из устаревшего кэша) без нужного заведения, но
  //    refetch ещё в процессе — иначе мелькнёт ложное «Заведение не найдено».
  if (isLoading || !venues || (!venue && isFetching)) {
    return (
      <div className="min-h-screen bg-muted/30">
        <div className="container mx-auto px-4 py-10">
          <div className="mx-auto max-w-4xl">
            <div className="h-40 animate-pulse rounded-lg border border-border bg-muted" />
          </div>
        </div>
      </div>
    );
  }

  // Свежий список загружен, но заведения в нём нет — реально не найдено.
  if (!venue) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-10">
        <p className="text-sm text-muted-foreground">Заведение не найдено.</p>
        <Button asChild variant="link" className="mt-2 h-auto px-0 text-sm">
          <Link href="/owner/venues">← Мои заведения</Link>
        </Button>
      </div>
    );
  }

  // Заведение найдено, но форма ещё инициализируется из него (эффект ниже) —
  // это загрузка, а не «не найдено».
  if (!form) {
    return (
      <div className="min-h-screen bg-muted/30">
        <div className="container mx-auto px-4 py-10">
          <div className="mx-auto max-w-4xl">
            <div className="h-40 animate-pulse rounded-lg border border-border bg-muted" />
          </div>
        </div>
      </div>
    );
  }

  const statusKey = (venue.status ?? "pending_review") as VenueStatusUI;
  const sc = statusConfig[statusKey] ?? statusConfig.pending_review;
  const StatusIcon = sc.icon;
  const statusMeta =
    VENUE_CARD_STATUS_LABELS[statusKey] ?? {
      label: statusKey,
      description: "",
    };
  const willReModerate =
    venue.status === "active" || venue.status === "rejected";
  const sortedPhotos = sortPhotos(venue.photos);
  const photoCount = sortedPhotos.length;
  const maxVenuePhotos = 24;
  const canAddPhotos = photoCount < maxVenuePhotos;

  const draftHasPhotos = photoCount > 0;
  const draftHallsPriced = form.halls.some(
    (h) => h.name.trim() !== "" && Number(h.price_from) > 0,
  );
  const draftDescOk = [...(form.description?.trim() ?? "")].length >= 40;
  const draftVerificationOk = draftVerificationOkForSubmit(form);
  const draftReadyForReview =
    draftHasPhotos &&
    draftHallsPriced &&
    draftDescOk &&
    draftVerificationOk;

  // Динамический чек-лист публикации (как у профиля мастера): пункты зеленеют по
  // мере заполнения. Показываем липким сайдбаром справа, только для черновика.
  const showVenueChecklist = venue.status === "draft";
  const venueChecklist = [
    { label: "Фото — хотя бы одно", done: draftHasPhotos },
    { label: "Цена за час по залам", done: draftHallsPriced },
    { label: "Описание — от ~2–3 предложений", done: draftDescOk },
    {
      label: "Данные для модерации: юрлицо/ИП, ИНН, ОГРН, ссылка на карты",
      done: draftVerificationOk,
    },
  ];
  const venueChecklistDone = venueChecklist.filter((i) => i.done).length;

  const baseline = baselineSerializedRef.current;
  const isDirty = Boolean(baseline && serializeForm(form) !== baseline);
  /** Отклонённую карточку можно отправить повторно без обязательных правок в форме */
  const canSave = isDirty || venue.status === "rejected";

  const moderatedDateLabel = venue.moderated_at
    ? new Date(venue.moderated_at).toLocaleDateString("ru-RU", {
        day: "numeric",
        month: "long",
        year: "numeric",
      })
    : null;

  const saveDisabled =
    mutation.isPending || persistVenueMutation.isPending || !canSave;

  const patchPartnerVenue = (patch: Partial<PartnerVenueFormValues>) => {
    setForm((prev) =>
      prev ? applyPartnerVenuePatchToUpdateForm(prev, patch) : prev,
    );
  };

  return (
    <section className="min-h-screen bg-muted/30 pb-28">
      <div className="sticky top-0 z-20 border-b border-border bg-background">
        <div className="container mx-auto px-4">
          <div className="flex min-h-16 flex-col gap-3 py-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex min-w-0 flex-1 flex-col gap-2 sm:flex-row sm:items-center sm:gap-4">
              <div className="flex flex-wrap items-center gap-2">
                <Button variant="ghost" size="sm" className="gap-2" asChild>
                  <Link href="/owner/venues">
                    <ArrowLeft className="h-4 w-4" />
                    Назад к списку
                  </Link>
                </Button>
                <Separator
                  orientation="vertical"
                  className="hidden h-6 sm:block"
                />
              </div>
              <div className="flex min-w-0 flex-wrap items-center gap-2 sm:gap-3">
                <h1 className="truncate text-lg font-semibold text-foreground">
                  {form.name || "Без названия"}
                </h1>
                <Badge className={`shrink-0 gap-1 ${sc.className}`}>
                  <StatusIcon className="h-3 w-3" />
                  {sc.label}
                </Badge>
              </div>
            </div>
            <div className="flex shrink-0 flex-wrap items-center gap-2 sm:gap-3">
              {venue.status !== "draft" ? (
                <Button variant="link" size="sm" className="h-auto px-0" asChild>
                  <Link
                    href={`/venues/${venue.slug}`}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    Как на сайте
                  </Link>
                </Button>
              ) : null}
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={handleCancel}
                disabled={mutation.isPending}
              >
                Отмена
              </Button>
              <Button
                type="submit"
                form="venue-edit-form"
                size="sm"
                disabled={saveDisabled}
              >
                {mutation.isPending ? "Сохранение…" : "Сохранить изменения"}
              </Button>
            </div>
          </div>
        </div>
      </div>

      <div className="container mx-auto px-4 py-8">
        <div
          className={
            showVenueChecklist
              ? "mx-auto max-w-6xl lg:grid lg:grid-cols-[minmax(0,1fr)_300px] lg:items-start lg:gap-8"
              : "mx-auto max-w-4xl"
          }
        >
          <div className="min-w-0 space-y-6">
          {statusMeta.description ? (
            <p className="text-sm text-muted-foreground">{statusMeta.description}</p>
          ) : null}

          {venue.moderation_comment ? (
            <Card className={`border-2 ${sc.bgClass}`}>
              <CardContent className="p-5">
                <div className="flex gap-4">
                  <div className="shrink-0">
                    {venue.status === "rejected" ? (
                      <div className="flex h-10 w-10 items-center justify-center rounded-full bg-red-100 dark:bg-red-950/50">
                        <XCircle className="h-5 w-5 text-red-600 dark:text-red-400" />
                      </div>
                    ) : venue.status === "suspended" ? (
                      <div className="flex h-10 w-10 items-center justify-center rounded-full bg-stone-200 dark:bg-stone-800">
                        <AlertTriangle className="h-5 w-5 text-stone-600 dark:text-stone-400" />
                      </div>
                    ) : (
                      <div className="flex h-10 w-10 items-center justify-center rounded-full bg-amber-100 dark:bg-amber-950/50">
                        <Info className="h-5 w-5 text-amber-600 dark:text-amber-400" />
                      </div>
                    )}
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="mb-1 flex flex-wrap items-center gap-2">
                      <span className="font-semibold text-foreground">
                        {venue.status === "rejected"
                          ? "Причина отклонения"
                          : venue.status === "suspended"
                            ? "Комментарий администратора"
                            : "Комментарий модератора"}
                      </span>
                      {moderatedDateLabel ? (
                        <span className="text-sm text-muted-foreground">
                          {moderatedDateLabel}
                        </span>
                      ) : null}
                    </div>
                    <p className="whitespace-pre-wrap text-sm leading-relaxed text-foreground/90">
                      {venue.moderation_comment}
                    </p>
                  </div>
                </div>
              </CardContent>
            </Card>
          ) : null}

          {venue.status === "rejected" && !venue.moderation_comment ? (
            <Card className="border-2 border-red-200 bg-red-50 dark:border-red-900 dark:bg-red-950/20">
              <CardContent className="p-5 text-sm text-red-900 dark:text-red-100">
                Карточка отклонена, текст причины не передан. Уточните детали в
                поддержке. После правок сохраните карточку, чтобы снова отправить
                на проверку.
              </CardContent>
            </Card>
          ) : null}

          {venue.status === "active" ? (
            <div className="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-950 dark:border-amber-900 dark:bg-amber-950/25 dark:text-amber-100">
              <strong className="font-semibold">Публикация:</strong> после
              сохранения изменений карточка снова уйдёт на модерацию и в каталоге
              может временно отображаться по прежним данным.
            </div>
          ) : null}

          {isDirty && willReModerate ? (
            <div className="flex gap-3 rounded-lg border border-amber-200 bg-amber-50 p-4 dark:border-amber-900 dark:bg-amber-950/25">
              <Info className="h-5 w-5 shrink-0 text-amber-600 dark:text-amber-400" />
              <p className="text-sm text-amber-950 dark:text-amber-100">
                После сохранения изменений карточка будет отправлена на повторную
                модерацию. Обычно проверка занимает до 24 часов.
              </p>
            </div>
          ) : null}

          {isDirty &&
          !willReModerate &&
          venue.status !== "suspended" &&
          venue.status === "pending_review" ? (
            <div className="flex gap-3 rounded-lg border border-border bg-muted/50 p-4">
              <Info className="h-5 w-5 shrink-0 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">
                Изменения сохранятся; статус останется «На модерации», пока
                модератор не проверит карточку.
              </p>
            </div>
          ) : null}

          <form id="venue-edit-form" onSubmit={handleSubmit} className="space-y-6">
            <PartnerVenueRegistrationCard
              values={partnerVenueValuesFromUpdateForm(form, venue.type)}
              onChange={patchPartnerVenue}
              showVenuePhone
              venuePhone={form.phone}
              onVenuePhoneChange={(v) => updateField("phone", v)}
              venueTypeReadOnly
              descriptionMaxLength={2000}
              wrapperClassName="max-w-none"
              slotAfterPhone={
                <>
                  <Separator />
                  <div>
                    <Label className="mb-3 block">Часы работы</Label>
                    <div className="grid gap-4 sm:grid-cols-2">
                      <div className="space-y-2">
                        <span className="text-sm text-muted-foreground">
                          Будни (Пн–Пт)
                        </span>
                        <div className="flex items-center gap-2">
                          <Input
                            type="time"
                            value={workingDraft.weekdays.from}
                            onChange={(e) =>
                              setWorkingDraft({
                                ...workingDraft,
                                weekdays: {
                                  ...workingDraft.weekdays,
                                  from: e.target.value,
                                },
                              })
                            }
                            className="min-w-0 flex-1"
                          />
                          <span className="text-muted-foreground">—</span>
                          <Input
                            type="time"
                            value={workingDraft.weekdays.to}
                            onChange={(e) =>
                              setWorkingDraft({
                                ...workingDraft,
                                weekdays: {
                                  ...workingDraft.weekdays,
                                  to: e.target.value,
                                },
                              })
                            }
                            className="min-w-0 flex-1"
                          />
                        </div>
                      </div>
                      <div className="space-y-2">
                        <span className="text-sm text-muted-foreground">
                          Выходные (Сб–Вс)
                        </span>
                        <div className="flex items-center gap-2">
                          <Input
                            type="time"
                            value={workingDraft.weekends.from}
                            onChange={(e) =>
                              setWorkingDraft({
                                ...workingDraft,
                                weekends: {
                                  ...workingDraft.weekends,
                                  from: e.target.value,
                                },
                              })
                            }
                            className="min-w-0 flex-1"
                          />
                          <span className="text-muted-foreground">—</span>
                          <Input
                            type="time"
                            value={workingDraft.weekends.to}
                            onChange={(e) =>
                              setWorkingDraft({
                                ...workingDraft,
                                weekends: {
                                  ...workingDraft.weekends,
                                  to: e.target.value,
                                },
                              })
                            }
                            className="min-w-0 flex-1"
                          />
                        </div>
                      </div>
                    </div>
                  </div>
                </>
              }
            />

            <Card className="border-border">
              <CardHeader className="pb-4">
                <CardTitle className="text-lg">Соцсети и мессенджеры</CardTitle>
                <CardDescription>
                  Укажите ссылки, чтобы гости могли написать вам напрямую. Поля
                  необязательны.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <VenueSocialLinksFields
                  value={form.social_links}
                  onChange={(next) => updateField("social_links", next)}
                />
              </CardContent>
            </Card>

            <OwnerSlotBlocksSection venueId={venueId} />

            <Card className="border-border">
              <CardHeader className="pb-4">
                <CardTitle className="text-lg">Залы</CardTitle>
                <CardDescription>
                  У каждого зала — цена за час, вместимость и свои удобства. На
                  витрине каталога показывается минимальная цена и объединение
                  удобств по всем залам. Фото можно добавить сразу: для нового
                  зала карточка сохранится автоматически перед загрузкой.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-8">
                {photoError ? (
                  <p className="text-sm text-destructive" role="alert">
                    {photoError}
                  </p>
                ) : null}
                {form.halls.map((hall, hallIndex) => {
                  const hallExtra = hall.amenities.filter(
                    (a) => !PRESET_LABEL_SET.has(a),
                  );
                  const hallPhotos = sortPhotos(
                    hall.photos as VenuePhoto[] | undefined,
                  );
                  const maxHallPhotos = 16;
                  const canPickHallPhoto = Boolean(hall.id || hall.name.trim());
                  return (
                    <div
                      key={hall.clientKey}
                      className="rounded-xl border border-border bg-card/30 p-4 sm:p-5"
                    >
                      <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
                        <h3 className="text-base font-semibold text-foreground">
                          Зал {hallIndex + 1}
                        </h3>
                        {form.halls.length > 1 ? (
                          <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            className="gap-1 text-destructive hover:text-destructive"
                            onClick={() => removeHall(hallIndex)}
                          >
                            <Trash2 className="h-4 w-4" />
                            Удалить зал
                          </Button>
                        ) : null}
                      </div>
                      <div className="grid gap-4 sm:grid-cols-2">
                        <div className="space-y-2 sm:col-span-2">
                          <Label>
                            Название зала{" "}
                            <span className="text-destructive">*</span>
                          </Label>
                          <Input
                            value={hall.name}
                            onChange={(e) =>
                              updateHall(hallIndex, {
                                name: e.target.value.slice(0, 255),
                              })
                            }
                            placeholder="Например: Русская парная"
                            required
                          />
                        </div>
                        <div className="space-y-2">
                          <Label>Цена (₽/час)</Label>
                          <Input
                            type="number"
                            min={0}
                            value={hall.price_from || ""}
                            onChange={(e) =>
                              updateHall(hallIndex, {
                                price_from: Number(e.target.value),
                              })
                            }
                          />
                        </div>
                        <div className="space-y-2">
                          <Label>Вместимость (чел.)</Label>
                          <Input
                            type="number"
                            min={1}
                            value={hall.capacity || ""}
                            onChange={(e) =>
                              updateHall(hallIndex, {
                                capacity: Number(e.target.value),
                              })
                            }
                          />
                        </div>
                      </div>
                      <div className="mt-5 space-y-3">
                        <Label className="text-muted-foreground">
                          Удобства в зале
                        </Label>
                        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
                          {PRESET_AMENITIES.map((amenity) => (
                            <label
                              key={`${hall.clientKey}-${amenity.id}`}
                              className="flex cursor-pointer items-center gap-3 rounded-lg border border-border p-3 transition-colors hover:bg-muted/50 has-[:checked]:border-primary has-[:checked]:bg-primary/5"
                            >
                              <Checkbox
                                checked={hall.amenities.includes(amenity.label)}
                                onCheckedChange={() =>
                                  toggleHallPresetAmenity(
                                    hallIndex,
                                    amenity.label,
                                  )
                                }
                              />
                              <span className="text-sm">{amenity.label}</span>
                            </label>
                          ))}
                        </div>
                        {hallExtra.length > 0 ? (
                          <div className="flex flex-wrap gap-2">
                            {hallExtra.map((a) => (
                              <span
                                key={`${hall.clientKey}-x-${a}`}
                                className="inline-flex items-center gap-1 rounded-md border border-border bg-muted/40 px-2 py-1 text-sm"
                              >
                                {a}
                                <button
                                  type="button"
                                  className="rounded p-0.5 hover:bg-background"
                                  onClick={() =>
                                    removeHallCustomAmenity(hallIndex, a)
                                  }
                                  aria-label={`Удалить «${a}»`}
                                >
                                  <X className="h-3.5 w-3.5" />
                                </button>
                              </span>
                            ))}
                          </div>
                        ) : null}
                        <div className="flex max-w-lg gap-2">
                          <Input
                            placeholder="Своё удобство…"
                            value={hallAmenityDraft[hall.clientKey] ?? ""}
                            onChange={(e) =>
                              setHallAmenityDraft((d) => ({
                                ...d,
                                [hall.clientKey]: e.target.value,
                              }))
                            }
                            onKeyDown={(e) => {
                              if (e.key === "Enter") {
                                e.preventDefault();
                                addHallCustomAmenity(hallIndex);
                              }
                            }}
                          />
                          <Button
                            type="button"
                            variant="outline"
                            size="icon"
                            onClick={() => addHallCustomAmenity(hallIndex)}
                          >
                            <Plus className="h-4 w-4" />
                          </Button>
                        </div>
                      </div>
                      <div className="mt-6 border-t border-border pt-5">
                        <Label className="mb-2 block">Фото зала</Label>
                        {!hall.id ? (
                          <p className="mb-3 text-sm text-muted-foreground">
                            Укажите название зала — при первой загрузке фото
                            карточка сохранится автоматически.
                          </p>
                        ) : null}
                        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
                          {hallPhotos.map((p) => (
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
                                  <Badge
                                    variant="secondary"
                                    className="text-xs"
                                  >
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
                                    disabled={
                                      coverHallPhotoMu.isPending || !hall.id
                                    }
                                    onClick={() =>
                                      coverHallPhotoMu.mutate({
                                        hallId: hall.id as string,
                                        photoId: p.id,
                                      })
                                    }
                                  >
                                    В обложку
                                  </Button>
                                ) : null}
                                <Button
                                  type="button"
                                  size="sm"
                                  variant="destructive"
                                  className="h-8 text-xs"
                                  disabled={
                                    deleteHallPhotoMu.isPending || !hall.id
                                  }
                                  onClick={() =>
                                    deleteHallPhotoMu.mutate({
                                      hallId: hall.id as string,
                                      photoId: p.id,
                                    })
                                  }
                                >
                                  Удалить
                                </Button>
                              </div>
                            </div>
                          ))}
                        </div>
                        {hallPhotos.length < maxHallPhotos ? (
                          <div className="mt-3">
                            <input
                              type="file"
                              accept="image/jpeg,image/png,image/webp"
                              className="hidden"
                              id={`hall-photo-${hall.clientKey}`}
                              onChange={async (e) => {
                                const f = e.target.files?.[0];
                                e.target.value = "";
                                if (!f || !form) return;
                                const h = form.halls[hallIndex];
                                if (!h) return;
                                setPhotoError("");
                                try {
                                  let hallId = h.id ?? "";
                                  if (!hallId) {
                                    const t = h.name.trim();
                                    if (!t) {
                                      setPhotoError(
                                        "Укажите название зала, чтобы загрузить фото.",
                                      );
                                      return;
                                    }
                                    if (
                                      !form.halls.some((x) => x.name.trim())
                                    ) {
                                      setPhotoError(
                                        "Укажите название хотя бы для одного зала.",
                                      );
                                      return;
                                    }
                                    const updated =
                                      await persistVenueMutation.mutateAsync(
                                        buildVenueUpdatePayload(form),
                                      );
                                    const merged =
                                      mergeHallIdsFromUpdatedVenue(
                                        form,
                                        updated,
                                      );
                                    setForm(merged);
                                    baselineSerializedRef.current =
                                      serializeForm(merged);
                                    hallId =
                                      merged.halls[hallIndex]?.id ?? "";
                                  }
                                  if (!hallId) {
                                    setPhotoError(
                                      "Не удалось сохранить зал. Сохраните карточку кнопкой «Сохранить изменения» и попробуйте снова.",
                                    );
                                    return;
                                  }
                                  uploadHallPhotoMu.mutate({
                                    hallId,
                                    file: f,
                                  });
                                } catch {
                                  /* persistVenueMutation.onError */
                                }
                              }}
                            />
                            <Button
                              type="button"
                              variant="outline"
                              size="sm"
                              className="gap-2"
                              disabled={
                                !canPickHallPhoto ||
                                uploadHallPhotoMu.isPending ||
                                persistVenueMutation.isPending
                              }
                              title={
                                !canPickHallPhoto
                                  ? "Сначала укажите название зала"
                                  : undefined
                              }
                              onClick={() =>
                                document
                                  .getElementById(
                                    `hall-photo-${hall.clientKey}`,
                                  )
                                  ?.click()
                              }
                            >
                              <Upload className="h-4 w-4" />
                              {persistVenueMutation.isPending
                                ? "Сохранение…"
                                : uploadHallPhotoMu.isPending
                                  ? "Загрузка…"
                                  : "Добавить фото"}
                            </Button>
                          </div>
                        ) : null}
                      </div>
                    </div>
                  );
                })}
                <Button
                  type="button"
                  variant="outline"
                  onClick={addHall}
                  className="gap-2"
                >
                  <Plus className="h-4 w-4" />
                  Добавить зал
                </Button>
              </CardContent>
            </Card>

            <VenueServicesFields
              value={form.services}
              onChange={(next) => updateField("services", next)}
              disabled={mutation.isPending}
            />

            <Card className="border-border">
              <CardHeader className="pb-4">
                <CardTitle className="text-lg">Фотографии</CardTitle>
                <CardDescription>
                  Это общие фотографии заведения для карточки в каталоге (не фото
                  отдельных залов — их можно добавить в блоке залов выше).
                  Рекомендуем не менее 3 снимков. JPEG, PNG или WebP, до 5 МБ каждый,
                  максимум {maxVenuePhotos}. Смена фото и обложки может отправить
                  карточку на повторную модерацию, если она была опубликована или
                  отклонена.
                </CardDescription>
              </CardHeader>
              <CardContent>
                {photoError ? (
                  <p className="mb-3 text-sm text-destructive" role="alert">
                    {photoError}
                  </p>
                ) : null}
                <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4">
                  {sortedPhotos.map((p) => (
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
                  {photoCount === 0 && venue.image_url ? (
                    <div className="relative aspect-square overflow-hidden rounded-lg border border-dashed border-border">
                      <Image
                        src={venueMediaUrl(venue.image_url)}
                        alt=""
                        fill
                        className="object-cover opacity-90"
                        sizes="(max-width: 768px) 50vw, 25vw"
                        unoptimized
                      />
                      <Badge
                        variant="outline"
                        className="absolute bottom-2 left-2 bg-background/90 text-xs"
                      >
                        Старое фото в каталоге
                      </Badge>
                    </div>
                  ) : null}
                  <label
                    className={
                      canAddPhotos && !uploadPhotoMu.isPending
                        ? "flex aspect-square cursor-pointer flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed border-border bg-muted/30 text-muted-foreground transition-colors hover:border-primary hover:bg-muted/50 hover:text-primary"
                        : "flex aspect-square cursor-not-allowed flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed border-border bg-muted/20 text-muted-foreground opacity-60"
                    }
                  >
                    <Upload className="h-8 w-8" />
                    <span className="px-2 text-center text-xs">
                      {uploadPhotoMu.isPending
                        ? "Загрузка…"
                        : canAddPhotos
                          ? "Загрузить"
                          : "Лимит фото"}
                    </span>
                    <input
                      type="file"
                      accept="image/jpeg,image/png,image/webp"
                      className="hidden"
                      multiple
                      disabled={!canAddPhotos || uploadPhotoMu.isPending}
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
                {photoCount < 3 ? (
                  <p className="mt-3 text-sm text-amber-700 dark:text-amber-500">
                    Загружено {photoCount} из 3 рекомендуемых фотографий
                  </p>
                ) : null}
                {photoCount >= maxVenuePhotos ? (
                  <p className="mt-2 text-sm text-muted-foreground">
                    Достигнут лимит {maxVenuePhotos} фото. Удалите лишние, чтобы
                    загрузить новые.
                  </p>
                ) : null}
              </CardContent>
            </Card>

            {error ? (
              <p className="text-sm text-destructive" role="alert">
                {error}
              </p>
            ) : null}

            <div className="sticky bottom-0 z-10 -mx-4 border-t border-border bg-background/95 px-4 py-4 backdrop-blur supports-[backdrop-filter]:bg-background/90 sm:-mx-0 sm:rounded-b-lg">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="text-sm text-muted-foreground">
                  {isDirty ? (
                    <span className="text-amber-700 dark:text-amber-500">
                      Есть несохранённые изменения
                    </span>
                  ) : (
                    <span>Нет несохранённых изменений</span>
                  )}
                </div>
                <div className="flex flex-wrap gap-3">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={handleCancel}
                    disabled={mutation.isPending}
                  >
                    Отмена
                  </Button>
                  <Button type="submit" disabled={saveDisabled}>
                    {mutation.isPending ? "Сохранение…" : "Сохранить изменения"}
                  </Button>
                </div>
              </div>
            </div>
          </form>
          </div>

          {showVenueChecklist ? (
            <aside className="mt-6 lg:mt-0 lg:sticky lg:top-20 lg:max-h-[calc(100vh-6rem)] lg:self-start lg:overflow-y-auto">
              <Card className="border-border">
                <CardHeader className="pb-3">
                  <CardTitle className="text-base">Чек-лист публикации</CardTitle>
                  <CardDescription>
                    Выполнено {venueChecklistDone} из {venueChecklist.length}.
                    Заполните все пункты, чтобы отправить карточку на модерацию.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <ul className="space-y-2.5">
                    {venueChecklist.map((item) => (
                      <li key={item.label} className="flex items-start gap-2.5 text-sm">
                        {item.done ? (
                          <CheckCircle2
                            className="mt-0.5 h-4 w-4 shrink-0 text-green-600 dark:text-green-500"
                            aria-hidden
                          />
                        ) : (
                          <Circle
                            className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground/40"
                            aria-hidden
                          />
                        )}
                        <span className={item.done ? "text-foreground" : "text-muted-foreground"}>
                          {item.label}
                        </span>
                      </li>
                    ))}
                  </ul>
                  {submitReviewError ? (
                    <p className="mt-4 text-sm text-destructive" role="alert">
                      {submitReviewError}
                    </p>
                  ) : null}
                  <Button
                    type="button"
                    className="mt-4 w-full"
                    onClick={() => submitReviewMu.mutate()}
                    disabled={!draftReadyForReview || submitReviewMu.isPending}
                    title={
                      !draftReadyForReview && !submitReviewMu.isPending
                        ? "Выполните все пункты чек-листа."
                        : undefined
                    }
                  >
                    {submitReviewMu.isPending ? "Отправка…" : "Отправить на проверку"}
                  </Button>
                </CardContent>
              </Card>
            </aside>
          ) : null}
        </div>
      </div>
    </section>
  );
}
