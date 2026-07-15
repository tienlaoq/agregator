"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
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
  deleteMasterVideo,
  getMyMasterProfile,
  patchMyMasterProfile,
  setMasterCoverPhoto,
  submitMasterForReview,
  uploadMasterPhoto,
  uploadMasterVideo,
  formatApiErrorMessage,
  isEmailNotVerifiedError,
  venueMediaUrl,
} from "@/lib/api";
import { EmailVerificationNotice } from "@/components/banya/email-verification-notice";
import type {
  MasterCredentialItem,
  MasterCredentialKind,
  MasterPhoto,
  MasterProfile,
  MasterServiceItem,
  MasterTravelExcludeZone,
} from "@/lib/types";
import { excludeZoneContainedInTravelRadius, haversineKm } from "@/lib/geo";
import { MASTER_PROFILE_STATUS_LABELS } from "@/lib/types";
import { MasterTravelBaseMap } from "@/components/banya/master-travel-base-map";
import { PhoneInput, getRawPhone, displayPhoneFromStored } from "@/components/banya/phone-input";
import {
  Plus,
  Trash2,
  Upload,
  CircleAlert,
  Award,
  CheckCircle2,
  Circle,
} from "lucide-react";

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

type CredentialLine = {
  key: string;
  kind: MasterCredentialKind;
  title: string;
  issuer: string;
  /** Год получения как строка для удобного ввода; пусто = не указан. */
  year: string;
};

function newCredentialLine(kind: MasterCredentialKind = "certificate"): CredentialLine {
  return { key: crypto.randomUUID(), kind, title: "", issuer: "", year: "" };
}

function credentialsFromProfile(items: MasterCredentialItem[]): CredentialLine[] {
  return items.map((x) => ({
    key: x.id || crypto.randomUUID(),
    kind: x.kind === "award" ? "award" : "certificate",
    title: x.title,
    issuer: x.issuer ?? "",
    year: x.year && x.year > 0 ? String(x.year) : "",
  }));
}

const MAX_CREDENTIALS = 50;

const MAX_MASTER_PHOTOS = 12;

/** Старое значение из БД (до миграции) и единый регистр slug для формы/API. */
function normalizePayoutLegalFormStored(raw: string | undefined): string {
  const s = (raw ?? "").trim().toLowerCase();
  if (s === "gph") return "individual";
  return s;
}

const MAX_TRAVEL_EXCLUDE_ZONES = 20;
const DEFAULT_EXCLUDE_ZONE_RADIUS_KM = 1;
const MIN_EXCLUDE_RADIUS_KM = 0.1;
const MAX_EXCLUDE_RADIUS_KM = 50;

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

type ChecklistItem = { label: string; done: boolean; optional?: boolean };

/** Пункты готовности профиля к модерации. Зеркалит обязательные проверки из
 *  masterSubmitValidationMessage, но как независимые булевы флаги — чтобы каждый
 *  пункт чек-листа подсвечивался отдельно. Авторитетный гейт на отправку
 *  остаётся в masterSubmitValidationMessage. */
function masterReadinessChecklist(
  body: Record<string, unknown>,
  photoCount: number,
): ChecklistItem[] {
  const digits = (v: unknown) => String(v ?? "").replace(/\D/g, "");
  const plf = String(body.payout_legal_form ?? "").trim().toLowerCase();
  const innLen = digits(body.payout_inn).length;
  const innOk = plf === "ooo" ? innLen === 10 : innLen === 12;
  let payoutExtraOk = true;
  if (plf === "ooo") {
    payoutExtraOk =
      digits(body.payout_kpp).length === 9 && digits(body.payout_ogrn).length === 13;
  } else if (plf === "ip") {
    payoutExtraOk = digits(body.payout_ogrnip).length === 15;
  }
  // Только данные получателя (юр. форма + ФИО/название + ИНН, и КПП/ОГРН(ИП) где
  // нужно). Банковский счёт / СБП указываются отдельно в ЛК → Финансы.
  const payoutOk =
    ["ip", "ooo", "individual", "self_employed"].includes(plf) &&
    String(body.payout_legal_name ?? "").trim() !== "" &&
    innOk &&
    payoutExtraOk;
  const bio = String(body.bio ?? "").trim();
  const svc = body.services;
  return [
    { label: "Имя на платформе", done: String(body.display_name ?? "").trim() !== "" },
    { label: "Город", done: String(body.city ?? "").trim() !== "" },
    { label: "Телефон", done: normalizeRussianMobileDigits(String(body.phone ?? "")) !== "" },
    { label: "О себе — от 20 символов", done: [...bio].length >= 20 },
    { label: "Хотя бы одна услуга", done: Array.isArray(svc) && svc.length > 0 },
    { label: "Данные получателя выплат", done: payoutOk },
    { label: "Фото профиля", done: photoCount > 0, optional: true },
  ];
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
  const legalName = String(body.payout_legal_name ?? "").trim();
  if (!legalName) return "Укажите ФИО или наименование получателя";
  const inn = String(body.payout_inn ?? "").replace(/\D/g, "");
  if (plf === "ooo" && inn.length !== 10) return "Для ООО ИНН должен содержать 10 цифр";
  if (["ip", "individual", "self_employed"].includes(plf) && inn.length !== 12) {
    return "ИНН должен содержать 12 цифр";
  }
  if (plf === "ooo" && String(body.payout_kpp ?? "").replace(/\D/g, "").length !== 9) {
    return "Для ООО КПП должен содержать 9 цифр";
  }
  if (plf === "ooo" && String(body.payout_ogrn ?? "").replace(/\D/g, "").length !== 13) {
    return "Для ООО ОГРН должен содержать 13 цифр";
  }
  if (plf === "ip" && String(body.payout_ogrnip ?? "").replace(/\D/g, "").length !== 15) {
    return "Для ИП ОГРНИП должен содержать 15 цифр";
  }
  // Банковский счёт / СБП больше не требуются в карточке — они настраиваются
  // в ЛК → Финансы (payment-service payout method) и там же гейтят выплату.
  const wf = String(body.work_format ?? "").toLowerCase();
  if (wf === "mobile" || wf === "both") {
    const travelKm = Number(body.travel_radius_km);
    const baseLat = Number(body.travel_base_latitude);
    const baseLon = Number(body.travel_base_longitude);
    if (Number.isFinite(travelKm) && travelKm > 0) {
      if (!Number.isFinite(baseLat) || !Number.isFinite(baseLon)) {
        return "Поставьте метку на Яндекс.Картах (или найдите адрес) — без неё нельзя задать зону выезда";
      }
    }
    const rawZones = body.travel_exclude_zones;
    if (Array.isArray(rawZones)) {
      if (rawZones.length > MAX_TRAVEL_EXCLUDE_ZONES) {
        return `Не более ${MAX_TRAVEL_EXCLUDE_ZONES} зон «куда не выезжаю»`;
      }
      for (let i = 0; i < rawZones.length; i++) {
        const row = rawZones[i];
        if (!row || typeof row !== "object" || Array.isArray(row)) {
          return `Зона исключения ${i + 1}: некорректные данные`;
        }
        const o = row as Record<string, unknown>;
        const id = String(o.id ?? "").trim();
        if (!id) return `Зона исключения ${i + 1}: укажите идентификатор (сохраните профиль заново, если поле пустое)`;
        const lat = Number(o.latitude);
        const lon = Number(o.longitude);
        if (!Number.isFinite(lat) || lat < -90 || lat > 90 || !Number.isFinite(lon) || lon < -180 || lon > 180) {
          return `Зона исключения ${i + 1}: некорректные координаты`;
        }
        const rk = Number(o.radius_km);
        if (Number.isFinite(rk) && rk > 0 && (rk < MIN_EXCLUDE_RADIUS_KM || rk > MAX_EXCLUDE_RADIUS_KM)) {
          return `Зона исключения ${i + 1}: радиус от ${MIN_EXCLUDE_RADIUS_KM} до ${MAX_EXCLUDE_RADIUS_KM} км`;
        }
        if (
          Number.isFinite(travelKm) &&
          travelKm > 0 &&
          Number.isFinite(baseLat) &&
          Number.isFinite(baseLon) &&
          !excludeZoneContainedInTravelRadius(baseLat, baseLon, travelKm, lat, lon, rk)
        ) {
          return `Зона исключения ${i + 1}: круг должен целиком лежать внутри зоны выезда (${travelKm} км от метки)`;
        }
      }
    }
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
  const [error, setError] = useState("");
  const [emailNotVerified, setEmailNotVerified] = useState(false);
  const [submitWarning, setSubmitWarning] = useState("");
  const [photoError, setPhotoError] = useState("");
  const [videoError, setVideoError] = useState("");

  const [displayName, setDisplayName] = useState("");
  const [bio, setBio] = useState("");
  const [phone, setPhone] = useState("");
  const [city, setCity] = useState("");
  const [payoutLegalForm, setPayoutLegalForm] = useState("");
  const [payoutLegalName, setPayoutLegalName] = useState("");
  const [payoutInn, setPayoutInn] = useState("");
  const [payoutKpp, setPayoutKpp] = useState("");
  const [payoutOgrn, setPayoutOgrn] = useState("");
  const [payoutOgrnip, setPayoutOgrnip] = useState("");
  const [workFormat, setWorkFormat] = useState("both");
  const [travelRadius, setTravelRadius] = useState(0);
  /** Координаты метки с Яндекс.Карт (на сервер уходят как travel_base_*). */
  const [travelBasePinLat, setTravelBasePinLat] = useState<number | null>(null);
  const [travelBasePinLon, setTravelBasePinLon] = useState<number | null>(null);
  const [travelExcludeZones, setTravelExcludeZones] = useState<MasterTravelExcludeZone[]>([]);
  const [excludePlacementMode, setExcludePlacementMode] = useState(false);
  const [experienceYears, setExperienceYears] = useState(0);
  const [hourlyRub, setHourlyRub] = useState(0);
  const [weekendRub, setWeekendRub] = useState(0);
  const [specText, setSpecText] = useState("");
  const [availabilityNote, setAvailabilityNote] = useState("");
  const [services, setServices] = useState<ServiceLine[]>([newServiceLine()]);
  const [credentials, setCredentials] = useState<CredentialLine[]>([]);
  // useState вместо useRef: ref нельзя писать во время рендера, а гидрация
  // теперь делается render-time (см. ниже), не в useEffect.
  const [hydratedProfileId, setHydratedProfileId] = useState<string | null>(null);
  // Одноразовый флаг: для нового мастера (профиля ещё нет) подставляем телефон
  // из аккаунта — он был указан при регистрации. Без этого блок гидрации из
  // профиля ниже не сработает (профиля нет), и номер пришлось бы вводить заново.
  const [newMasterPhoneSeeded, setNewMasterPhoneSeeded] = useState(false);

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
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });

  const profile =
    profileQuery.data === null || profileQuery.data === undefined
      ? undefined
      : profileQuery.data;
  const notFound = profileQuery.isSuccess && profileQuery.data === null;

  const yandexMapsApiKey = process.env.NEXT_PUBLIC_YANDEX_MAPS_API_KEY;
  const travelMapVersion = useMemo(
    () => `${profile?.updated_at ?? "none"}|${workFormat}|${city.trim().slice(0, 80)}`,
    [profile?.updated_at, workFormat, city],
  );

  const onTravelBaseMapPosition = useCallback((lat: number, lon: number) => {
    setTravelBasePinLat(lat);
    setTravelBasePinLon(lon);
  }, []);

  const onAddExclusionFromMap = useCallback(
    (lat: number, lon: number) => {
      if (travelExcludeZones.length >= MAX_TRAVEL_EXCLUDE_ZONES) return;
      const pinLat = travelBasePinLat;
      const pinLon = travelBasePinLon;
      if (
        pinLat == null ||
        pinLon == null ||
        !Number.isFinite(pinLat) ||
        !Number.isFinite(pinLon)
      ) {
        setError("Сначала поставьте метку на карте и задайте километраж выезда.");
        return;
      }
      if (!Number.isFinite(travelRadius) || travelRadius <= 0) {
        setError("Укажите километраж выезда от метки — зона исключения должна помещаться внутрь него.");
        return;
      }
      if (
        !excludeZoneContainedInTravelRadius(
          pinLat,
          pinLon,
          travelRadius,
          lat,
          lon,
          DEFAULT_EXCLUDE_ZONE_RADIUS_KM,
        )
      ) {
        setError(
          "Круг «куда не выезжаю» должен целиком лежать внутри синей зоны выезда. Кликните ближе к метке, увеличьте километраж выезда или уменьшите радиус зоны после добавления.",
        );
        return;
      }
      setError("");
      setTravelExcludeZones((prev) => [
        ...prev,
        {
          id: crypto.randomUUID(),
          latitude: lat,
          longitude: lon,
          radius_km: DEFAULT_EXCLUDE_ZONE_RADIUS_KM,
          label: "",
        },
      ]);
      setExcludePlacementMode(false);
    },
    [
      travelBasePinLat,
      travelBasePinLon,
      travelRadius,
      travelExcludeZones.length,
    ],
  );

  // Render-time гидрация формы из профиля (React 19 pattern): однократно
  // при смене profile.id заполняем все поля формы. Заменяет useEffect, чтобы
  // не нарушать react-hooks/set-state-in-effect.
  if (profile && hydratedProfileId !== profile.id) {
    setHydratedProfileId(profile.id);

    setDisplayName(profile.display_name);
    setBio(profile.bio);
    const masterPhone = profile.phone?.trim() ?? "";
    setPhone(
      displayPhoneFromStored(masterPhone || (user?.role === "master" ? user.phone : "") || ""),
    );
    setCity(profile.city);
    setPayoutLegalForm(normalizePayoutLegalFormStored(profile.payout_legal_form));
    setPayoutLegalName(profile.payout_legal_name ?? "");
    setPayoutInn(profile.payout_inn ?? "");
    setPayoutKpp(profile.payout_kpp ?? "");
    setPayoutOgrn(profile.payout_ogrn ?? "");
    setPayoutOgrnip(profile.payout_ogrnip ?? "");
    setWorkFormat(profile.work_format || "both");
    setTravelRadius(profile.travel_radius_km);
    const pLat = profile.travel_base_latitude;
    const pLon = profile.travel_base_longitude;
    if (pLat != null && pLon != null && Number.isFinite(pLat) && Number.isFinite(pLon)) {
      setTravelBasePinLat(pLat);
      setTravelBasePinLon(pLon);
    } else {
      setTravelBasePinLat(null);
      setTravelBasePinLon(null);
    }
    setExperienceYears(profile.experience_years);
    setHourlyRub(kopecksToRub(profile.hourly_rate));
    setWeekendRub(kopecksToRub(profile.price_weekend));
    setSpecText(profile.specializations?.join(", ") ?? "");
    setAvailabilityNote(availabilityNoteFromStored(profile.availability_json));
    setServices(
      profile.services?.length ? servicesFromProfile(profile.services) : [newServiceLine()],
    );
    setCredentials(
      profile.credentials?.length ? credentialsFromProfile(profile.credentials) : [],
    );
    const wf = profile.work_format || "both";
    if (wf === "mobile" || wf === "both") {
      setTravelExcludeZones(
        (profile.travel_exclude_zones ?? []).map((z) => ({
          id: z.id?.trim() ? z.id : crypto.randomUUID(),
          latitude: z.latitude,
          longitude: z.longitude,
          radius_km: z.radius_km,
          label: typeof z.label === "string" ? z.label : "",
        })),
      );
    } else {
      setTravelExcludeZones([]);
    }
    setExcludePlacementMode(false);
  }

  // Новый мастер (сохранённого профиля ещё нет): телефон, указанный при
  // регистрации, лежит на аккаунте. Подставляем его один раз, чтобы не вводить
  // повторно. Блок гидрации выше для этого случая не запускается (profile = null).
  if (notFound && !newMasterPhoneSeeded) {
    setNewMasterPhoneSeeded(true);
    const registeredPhone = user?.role === "master" ? user.phone?.trim() : "";
    if (registeredPhone) {
      setPhone(displayPhoneFromStored(registeredPhone));
    }
  }

  // Имя берём из регистрации (user.name) — отдельно спрашивать его на этом шаге
  // не нужно. Его всегда можно изменить ниже в поле «Имя на платформе».
  const defaultMasterName = (
    user?.name?.trim() ||
    user?.email?.split("@")[0] ||
    "Мастер"
  ).trim();

  const createMut = useMutation({
    mutationFn: () => createMasterProfile(defaultMasterName),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["my-master-profile"] });
      setError("");
      setEmailNotVerified(false);
    },
    onError: (e) => {
      // Email-gate: show the resend block instead of the generic error.
      if (isEmailNotVerifiedError(e)) {
        setEmailNotVerified(true);
        setError("");
      } else {
        setEmailNotVerified(false);
        setError(formatApiErrorMessage(e, "Не удалось создать профиль"));
      }
    },
  });

  // Профиль создаётся автоматически, как только выясняется, что его ещё нет:
  // имя уже известно из регистрации, поэтому повторный ввод не нужен. isIdle
  // не даёт повторно дёргать создание после ошибки (в т.ч. email-gate).
  useEffect(() => {
    if (notFound && !emailNotVerified && createMut.isIdle) {
      createMut.mutate();
    }
  }, [notFound, emailNotVerified, createMut]);

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
    const credPayload = credentials
      .filter((c) => c.title.trim())
      .map((c) => {
        const yearNum = parseInt(c.year, 10);
        return {
          kind: c.kind,
          title: c.title.trim(),
          issuer: c.issuer.trim(),
          year: Number.isFinite(yearNum) && yearNum > 0 ? yearNum : 0,
        };
      });
    const body: Record<string, unknown> = {
      display_name: displayName.trim(),
      bio: bio.trim(),
      phone: getRawPhone(phone).trim(),
      city: city.trim(),
      payout_legal_form: normalizePayoutLegalFormStored(payoutLegalForm),
      payout_legal_name: payoutLegalName.trim(),
      payout_inn: payoutInn.trim(),
      payout_kpp: payoutKpp.trim(),
      payout_ogrn: payoutOgrn.trim(),
      payout_ogrnip: payoutOgrnip.trim(),
      work_format: workFormat,
      travel_radius_km: travelRadius,
      experience_years: experienceYears,
      hourly_rate: rubToKopecks(hourlyRub),
      price_weekend: rubToKopecks(weekendRub),
      availability_json: mergeAvailabilityWithNote(profile?.availability_json, availabilityNote),
      specializations: specs,
      services: svcPayload,
      credentials: credPayload,
    };
    if (workFormat === "mobile" || workFormat === "both") {
      const lat = travelBasePinLat;
      const lon = travelBasePinLon;
      if (lat != null && lon != null && Number.isFinite(lat) && Number.isFinite(lon)) {
        body.travel_base_latitude = lat;
        body.travel_base_longitude = lon;
      }
      body.travel_exclude_zones = travelExcludeZones.map((z) => ({
        id: z.id,
        latitude: z.latitude,
        longitude: z.longitude,
        radius_km: z.radius_km,
        label: z.label.trim(),
      }));
    }
    return body;
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

  // Render-time переоценка submitWarning по мере редактирования формы.
  // Раньше: useEffect с длинным списком зависимостей — нарушало
  // react-hooks/set-state-in-effect. Теперь считаем актуальное предупреждение
  // на каждом рендере (только когда уже показано) и синхронизируем в state
  // через render-time setState (React 19 pattern).
  const liveSubmitWarning = submitWarning
    ? (masterSubmitValidationMessage(buildMasterPatchBody()) ?? "")
    : "";
  const [prevLiveSubmitWarning, setPrevLiveSubmitWarning] = useState(submitWarning);
  if (submitWarning && liveSubmitWarning !== prevLiveSubmitWarning) {
    setPrevLiveSubmitWarning(liveSubmitWarning);
    setSubmitWarning(liveSubmitWarning);
  }

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

  const uploadVideoMu = useMutation({
    mutationFn: (file: File) => uploadMasterVideo(file),
    onSuccess: () => {
      setVideoError("");
      void qc.invalidateQueries({ queryKey: ["my-master-profile"] });
    },
    onError: (e: unknown) =>
      setVideoError(
        formatApiErrorMessage(
          e,
          "Не удалось загрузить видео. Допустимы MP4 и WebM, до 50 МБ, не более 6 роликов.",
        ),
      ),
  });

  const deleteVideoMu = useMutation({
    mutationFn: (videoId: string) => deleteMasterVideo(videoId),
    onSuccess: () => {
      setVideoError("");
      void qc.invalidateQueries({ queryKey: ["my-master-profile"] });
    },
    onError: (e: unknown) =>
      setVideoError(formatApiErrorMessage(e, "Не удалось удалить видео.")),
  });

  if (!hydrated || !token || user?.role !== "master") return null;

  const meta = profile ? MASTER_PROFILE_STATUS_LABELS[profile.status] : null;
  const canSubmit =
    profile &&
    ["draft", "needs_revision", "rejected"].includes(profile.status);

  const sortedMasterPhotos = profile ? sortMasterPhotos(profile.photos) : [];
  const masterPhotoCount = sortedMasterPhotos.length;
  const canAddMasterPhotos = masterPhotoCount < MAX_MASTER_PHOTOS;

  const sortedMasterVideos = [...(profile?.videos ?? [])].sort(
    (a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0),
  );
  const masterVideoCount = sortedMasterVideos.length;
  const MAX_MASTER_VIDEOS = 6;
  const canAddMasterVideos = masterVideoCount < MAX_MASTER_VIDEOS;

  const checklistBody = buildMasterPatchBody();
  const checklist = masterReadinessChecklist(checklistBody, masterPhotoCount);
  const checklistDone = checklist.filter((i) => i.done).length;
  // Ready when every required (non-optional) checklist item is done — mirrors the
  // venue editor's draftReadyForReview gate on the submit button.
  const masterReadyForReview = checklist.every((i) => i.optional || i.done);

  return (
    <div className="container mx-auto max-w-6xl px-4 py-10">
      <h1 className="mb-2 text-2xl font-bold">Профиль мастера</h1>
      <p className="mb-6 text-muted-foreground">
        До одобрения модератором профиль не показывается клиентам в каталоге.
      </p>

      {error && <p className="mb-4 text-sm text-destructive">{error}</p>}

      {profileQuery.isLoading && <p className="text-muted-foreground">Загрузка...</p>}

      {notFound && (
        <Card>
          <CardContent className="space-y-4 py-6">
            {emailNotVerified && user?.email ? (
              <EmailVerificationNotice email={user.email} />
            ) : error ? (
              <Button onClick={() => createMut.mutate()} disabled={createMut.isPending}>
                {createMut.isPending ? "Создание..." : "Повторить"}
              </Button>
            ) : (
              <p className="text-muted-foreground">Готовим ваш профиль…</p>
            )}
          </CardContent>
        </Card>
      )}

      {!profileQuery.isLoading && profile && (
        <div className="lg:grid lg:grid-cols-[minmax(0,1fr)_300px] lg:items-start lg:gap-8">
          <div className="min-w-0">
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

          <Card className="mb-6">
            <CardHeader className="pb-4">
              <CardTitle className="text-lg">Видео</CardTitle>
              <CardDescription>
                Короткие ролики о вашей работе. MP4 или WebM, до 50 МБ каждый,
                не более {MAX_MASTER_VIDEOS}.
              </CardDescription>
            </CardHeader>
            <CardContent>
              {videoError ? (
                <p className="mb-3 text-sm text-destructive" role="alert">
                  {videoError}
                </p>
              ) : null}
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {sortedMasterVideos.map((v) => (
                  <div
                    key={v.id}
                    className="group relative overflow-hidden rounded-lg border border-border"
                  >
                    <video
                      src={venueMediaUrl(v.url)}
                      controls
                      preload="metadata"
                      className="aspect-video w-full bg-black object-contain"
                    />
                    <Button
                      type="button"
                      size="sm"
                      variant="destructive"
                      className="absolute right-2 top-2 h-8 px-2 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
                      disabled={deleteVideoMu.isPending}
                      onClick={() => {
                        if (
                          window.confirm(
                            "Удалить это видео? Его нельзя будет восстановить.",
                          )
                        ) {
                          deleteVideoMu.mutate(v.id);
                        }
                      }}
                      aria-label="Удалить видео"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                ))}
                <label
                  className={
                    canAddMasterVideos && !uploadVideoMu.isPending
                      ? "flex aspect-video cursor-pointer flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed border-border bg-muted/30 text-muted-foreground transition-colors hover:border-primary hover:bg-muted/50 hover:text-primary"
                      : "flex aspect-video cursor-not-allowed flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed border-border bg-muted/20 text-muted-foreground opacity-60"
                  }
                >
                  <Upload className="h-8 w-8" />
                  <span className="px-2 text-center text-xs">
                    {uploadVideoMu.isPending
                      ? "Загрузка…"
                      : canAddMasterVideos
                        ? "Загрузить видео"
                        : "Лимит видео"}
                  </span>
                  <input
                    type="file"
                    accept="video/mp4,video/webm"
                    className="hidden"
                    multiple
                    disabled={!canAddMasterVideos || uploadVideoMu.isPending}
                    onChange={async (e) => {
                      const { files } = e.target;
                      if (!files?.length) return;
                      for (const f of Array.from(files)) {
                        try {
                          await uploadVideoMu.mutateAsync(f);
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
                  <Label htmlFor="masterPhone">Телефон для клиентов</Label>
                  <p className="text-xs text-muted-foreground">
                    Показывается в каталоге и на вашей странице; для входа в аккаунт используется email.
                  </p>
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
              <div id="payouts" className="space-y-2 scroll-mt-24">
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
              <div className="grid gap-4 rounded-md border border-border/70 p-3 sm:grid-cols-2">
                <div className="space-y-2 sm:col-span-2">
                  <Label>ФИО / наименование получателя</Label>
                  <Input value={payoutLegalName} onChange={(e) => setPayoutLegalName(e.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label>ИНН</Label>
                  <Input value={payoutInn} onChange={(e) => setPayoutInn(e.target.value)} />
                </div>
                {payoutLegalForm === "ooo" ? (
                  <div className="space-y-2">
                    <Label>КПП</Label>
                    <Input value={payoutKpp} onChange={(e) => setPayoutKpp(e.target.value)} />
                  </div>
                ) : null}
                {payoutLegalForm === "ooo" ? (
                  <div className="space-y-2">
                    <Label>ОГРН</Label>
                    <Input value={payoutOgrn} onChange={(e) => setPayoutOgrn(e.target.value)} />
                  </div>
                ) : null}
                {payoutLegalForm === "ip" ? (
                  <div className="space-y-2">
                    <Label>ОГРНИП</Label>
                    <Input value={payoutOgrnip} onChange={(e) => setPayoutOgrnip(e.target.value)} />
                  </div>
                ) : null}
                <p className="text-xs text-muted-foreground sm:col-span-2">
                  Куда переводить деньги — банковский счёт или СБП — укажите в разделе{" "}
                  <Link
                    href="/owner/master/finance"
                    className="font-medium text-primary underline underline-offset-2"
                  >
                    ЛК → Финансы
                  </Link>
                  . Там же настраивается автоматическая выплата.
                </p>
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
              {(workFormat === "mobile" || workFormat === "both") && (
                <div className="space-y-4 rounded-lg border border-border bg-muted/20 p-4">
                  <div className="space-y-2">
                    <Label>Зона выезда на Яндекс.Картах</Label>
                    <p className="text-xs text-muted-foreground">
                      Сначала укажите точку на карте (поиск по адресу, перетаскивание метки или клик). От
                      этой точки платформа считает расстояние до клиента. Если метка ещё не сохранена,
                      карта подстраивается под поле «Город» выше.
                    </p>
                    <MasterTravelBaseMap
                      key={travelMapVersion}
                      apiKey={yandexMapsApiKey}
                      mapVersion={travelMapVersion}
                      cityHint={city.trim()}
                      seedLat={travelBasePinLat}
                      seedLon={travelBasePinLon}
                      onPositionChange={onTravelBaseMapPosition}
                      travelRadiusKm={travelRadius}
                      excludeZones={travelExcludeZones}
                      excludePlacementActive={excludePlacementMode}
                      onAddExclusionAt={onAddExclusionFromMap}
                    />
                  </div>
                  <div className="space-y-2 border-t border-border pt-4">
                    <Label htmlFor="travelKmFromPin">На сколько километров от метки принимаете выезды</Label>
                    <p className="text-xs text-muted-foreground">
                      Число километров по прямой на карте от вашей метки до точки клиента (как ориентир;
                      дорога может быть длиннее). Поле можно оставить пустым.
                    </p>
                    <Input
                      id="travelKmFromPin"
                      type="number"
                      min={0}
                      inputMode="numeric"
                      value={numFieldValue(travelRadius, true)}
                      onChange={(e) =>
                        setTravelRadius(parseNonNegIntInput(e.target.value, 0))
                      }
                    />
                  </div>
                  <div className="space-y-3 border-t border-border pt-4">
                    <div>
                      <Label>Куда не выезжаю</Label>
                      <p className="text-xs text-muted-foreground">
                        Внутри синей зоны выезда можно отметить круги «сюда не приезжаю» (например, закрытая
                        территория). Каждый такой круг должен целиком помещаться в синюю зону: от метки до
                        дальней точки круга не дальше, чем ваш километраж выезда. Радиус зоны можно оставить
                        пустым; если задаёте число, диапазон — от{" "}
                        {MIN_EXCLUDE_RADIUS_KM} до {MAX_EXCLUDE_RADIUS_KM} км, не более{" "}
                        {MAX_TRAVEL_EXCLUDE_ZONES} зон.
                      </p>
                    </div>
                    {travelExcludeZones.length > 0 ? (
                      <ul className="space-y-3">
                        {travelExcludeZones.map((z) => (
                          <li
                            key={z.id}
                            className="space-y-2 rounded-md border border-border bg-background/60 p-3"
                          >
                            <div className="flex flex-wrap items-end justify-between gap-2">
                              <p className="text-xs text-muted-foreground">
                                {z.latitude.toFixed(5)}, {z.longitude.toFixed(5)}
                              </p>
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                className="h-8 shrink-0 gap-1 text-destructive hover:text-destructive"
                                onClick={() =>
                                  setTravelExcludeZones((list) =>
                                    list.filter((x) => x.id !== z.id),
                                  )
                                }
                              >
                                <Trash2 className="h-4 w-4" aria-hidden />
                                Убрать
                              </Button>
                            </div>
                            <div className="space-y-1">
                              <Label className="text-xs" htmlFor={`excl-label-${z.id}`}>
                                Подпись (необязательно)
                              </Label>
                              <Input
                                id={`excl-label-${z.id}`}
                                value={z.label}
                                onChange={(e) => {
                                  const t = e.target.value;
                                  setTravelExcludeZones((list) =>
                                    list.map((x) => (x.id === z.id ? { ...x, label: t } : x)),
                                  );
                                }}
                                placeholder="Например: промзона у ТЭЦ"
                              />
                            </div>
                            <div className="space-y-1">
                              <Label className="text-xs" htmlFor={`excl-r-${z.id}`}>
                                Радиус зоны, км
                              </Label>
                              <Input
                                id={`excl-r-${z.id}`}
                                type="number"
                                min={MIN_EXCLUDE_RADIUS_KM}
                                max={MAX_EXCLUDE_RADIUS_KM}
                                step={0.1}
                                inputMode="decimal"
                                value={numFieldValue(z.radius_km, true)}
                                onChange={(e) => {
                                  const v = parseNonNegFloatInput(e.target.value, 0);
                                  setTravelExcludeZones((list) =>
                                    list.map((x) =>
                                      x.id === z.id ? { ...x, radius_km: v } : x,
                                    ),
                                  );
                                }}
                                onBlur={() => {
                                  setTravelExcludeZones((list) =>
                                    list.map((x) => {
                                      if (x.id !== z.id) return x;
                                      let r = x.radius_km;
                                      if (!Number.isFinite(r) || r <= 0) return { ...x, radius_km: 0 };
                                      if (r < MIN_EXCLUDE_RADIUS_KM) r = MIN_EXCLUDE_RADIUS_KM;
                                      if (r > MAX_EXCLUDE_RADIUS_KM) {
                                        r = MAX_EXCLUDE_RADIUS_KM;
                                      }
                                      const plat = travelBasePinLat;
                                      const plon = travelBasePinLon;
                                      if (
                                        plat != null &&
                                        plon != null &&
                                        Number.isFinite(plat) &&
                                        Number.isFinite(plon) &&
                                        Number.isFinite(travelRadius) &&
                                        travelRadius > 0
                                      ) {
                                        const d = haversineKm(plat, plon, x.latitude, x.longitude);
                                        const maxRInTravel = travelRadius - d;
                                        if (Number.isFinite(maxRInTravel)) {
                                          r = Math.min(r, maxRInTravel);
                                          if (r < MIN_EXCLUDE_RADIUS_KM) {
                                            r = MIN_EXCLUDE_RADIUS_KM;
                                          }
                                        }
                                      }
                                      return { ...x, radius_km: r };
                                    }),
                                  );
                                }}
                              />
                            </div>
                          </li>
                        ))}
                      </ul>
                    ) : (
                      <p className="text-xs text-muted-foreground">Пока нет зон исключения.</p>
                    )}
                    <Button
                      type="button"
                      variant={excludePlacementMode ? "default" : "outline"}
                      size="sm"
                      className="w-full sm:w-auto"
                      disabled={travelExcludeZones.length >= MAX_TRAVEL_EXCLUDE_ZONES}
                      onClick={() => setExcludePlacementMode((v) => !v)}
                    >
                      {excludePlacementMode
                        ? "Отменить: клик по карте"
                        : "Добавить зону кликом по карте"}
                    </Button>
                    {excludePlacementMode ? (
                      <p className="text-xs font-medium text-primary" role="status">
                        Щёлкните по карте в центре зоны, куда не выезжаете. Метку базы при этом не
                        сдвигаем.
                      </p>
                    ) : null}
                    {travelExcludeZones.length >= MAX_TRAVEL_EXCLUDE_ZONES ? (
                      <p className="text-xs text-muted-foreground">
                        Достигнут лимит {MAX_TRAVEL_EXCLUDE_ZONES} зон.
                      </p>
                    ) : null}
                  </div>
                </div>
              )}
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2 sm:col-span-2">
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
                <Label>Ставка по будням (₽/час)</Label>
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
                <Label>Ставка по выходным (₽/час)</Label>
                <p className="text-xs text-muted-foreground">
                  Применяется к броням на субботу и воскресенье. Оставьте 0 — будет как в будни.
                </p>
                <Input
                  type="number"
                  min={0}
                  step={100}
                  inputMode="decimal"
                  value={numFieldValue(weekendRub, true)}
                  onChange={(e) =>
                    setWeekendRub(parseNonNegFloatInput(e.target.value, 0))
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

              <div className="space-y-3">
                <div className="flex items-center justify-between gap-2">
                  <div className="flex items-center gap-2">
                    <Award className="h-4 w-4 text-muted-foreground" />
                    <Label>Сертификаты и награды</Label>
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="gap-1"
                    disabled={credentials.length >= MAX_CREDENTIALS}
                    onClick={() => setCredentials((c) => [...c, newCredentialLine()])}
                  >
                    <Plus className="h-4 w-4" />
                    Добавить
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  Необязательно. Дипломы, сертификаты об обучении и награды повышают
                  доверие клиентов — они отображаются в вашей карточке.
                </p>
                {credentials.length === 0 ? (
                  <p className="text-sm text-muted-foreground">Пока ничего не добавлено.</p>
                ) : (
                  credentials.map((line, idx) => (
                    <div key={line.key} className="space-y-2 rounded-lg border border-border p-3">
                      <div className="flex items-center justify-between gap-2">
                        <Select
                          value={line.kind}
                          onValueChange={(v) =>
                            setCredentials((c) =>
                              c.map((x, i) =>
                                i === idx ? { ...x, kind: v as MasterCredentialKind } : x,
                              ),
                            )
                          }
                        >
                          <SelectTrigger className="h-9 w-[170px]">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="certificate">Сертификат</SelectItem>
                            <SelectItem value="award">Награда</SelectItem>
                          </SelectContent>
                        </Select>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8 text-destructive"
                          onClick={() =>
                            setCredentials((c) => c.filter((_, i) => i !== idx))
                          }
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                      <Input
                        placeholder={
                          line.kind === "award"
                            ? "Название награды"
                            : "Название сертификата / диплома"
                        }
                        value={line.title}
                        onChange={(e) =>
                          setCredentials((c) =>
                            c.map((x, i) =>
                              i === idx ? { ...x, title: e.target.value } : x,
                            ),
                          )
                        }
                      />
                      <div className="grid grid-cols-1 gap-2 sm:grid-cols-[1fr_110px]">
                        <Input
                          placeholder="Кем выдан (необязательно)"
                          value={line.issuer}
                          onChange={(e) =>
                            setCredentials((c) =>
                              c.map((x, i) =>
                                i === idx ? { ...x, issuer: e.target.value } : x,
                              ),
                            )
                          }
                        />
                        <Input
                          inputMode="numeric"
                          placeholder="Год"
                          value={line.year}
                          onChange={(e) =>
                            setCredentials((c) =>
                              c.map((x, i) =>
                                i === idx
                                  ? { ...x, year: e.target.value.replace(/\D/g, "").slice(0, 4) }
                                  : x,
                              ),
                            )
                          }
                        />
                      </div>
                    </div>
                  ))
                )}
              </div>

              <div className="flex flex-col gap-3 pt-4 sm:flex-row sm:justify-end">
                <Button
                  disabled={saveMut.isPending}
                  onClick={() => saveMut.mutate()}
                >
                  {saveMut.isPending ? "Сохранение…" : "Сохранить изменения"}
                </Button>
              </div>
              {profile.status === "pending_review" && (
                <p className="text-sm text-muted-foreground">
                  Профиль на проверке. После решения модератора статус обновится здесь.
                </p>
              )}
            </CardContent>
          </Card>
          </div>

          <aside className="mt-6 lg:mt-0 lg:sticky lg:top-20 lg:max-h-[calc(100vh-6rem)] lg:self-start lg:overflow-y-auto">
            <Card className="border-border">
              <CardHeader className="pb-3">
                <CardTitle className="text-base">Чек-лист профиля</CardTitle>
                <CardDescription>
                  Выполнено {checklistDone} из {checklist.length}. Заполните
                  обязательные пункты, чтобы отправить профиль на модерацию.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <ul className="space-y-2.5">
                  {checklist.map((item) => (
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
                        {item.optional ? (
                          <span className="text-muted-foreground"> · необязательно</span>
                        ) : null}
                      </span>
                    </li>
                  ))}
                </ul>
                {canSubmit ? (
                  <>
                    {submitWarning ? (
                      <p
                        role="alert"
                        className="mt-4 text-sm leading-snug text-muted-foreground"
                      >
                        <CircleAlert
                          className="mr-1.5 inline-block size-3.5 shrink-0 align-[-0.15em] text-amber-600/85 dark:text-amber-500/85"
                          aria-hidden
                        />
                        {submitWarning}
                      </p>
                    ) : null}
                    <Button
                      type="button"
                      className="mt-4 w-full"
                      onClick={handleSubmitForReview}
                      disabled={
                        !masterReadyForReview ||
                        submitMut.isPending ||
                        saveMut.isPending
                      }
                      title={
                        !masterReadyForReview && !submitMut.isPending
                          ? "Выполните обязательные пункты чек-листа."
                          : undefined
                      }
                    >
                      {submitMut.isPending ? "Отправка…" : "Отправить на проверку"}
                    </Button>
                  </>
                ) : null}
              </CardContent>
            </Card>
          </aside>
        </div>
      )}
    </div>
  );
}
