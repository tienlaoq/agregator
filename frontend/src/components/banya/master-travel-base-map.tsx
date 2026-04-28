"use client";

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { MasterTravelExcludeZone } from "@/lib/types";
import {
  loadYandexMapsScript,
  type YCircle,
  type YCoord,
  type YMap,
  type YPlacemark,
} from "@/lib/yandex-maps-api";

const DEFAULT_CENTER: YCoord = [55.751574, 37.573856];

/** Круги не перехватывают клики — иначе нельзя поставить метку по карте. @see interactivityModel.storage */
const CIRCLE_NON_INTERACTIVE = { interactivityModel: "default#silent" as const };

const SERVICE_ZONE_STYLE = {
  ...CIRCLE_NON_INTERACTIVE,
  fillColor: "rgba(0, 119, 255, 0.14)",
  strokeColor: "#0066cc",
  strokeWidth: 2,
  strokeOpacity: 0.85,
  zIndex: 400,
};

const EXCLUDE_ZONE_BASE_STYLE = {
  ...CIRCLE_NON_INTERACTIVE,
  fillColor: "rgba(220, 38, 38, 0.22)",
  strokeColor: "#b91c1c",
  strokeWidth: 2,
  strokeOpacity: 0.9,
};

function safeRemoveFromMap(map: YMap, obj: YCircle | null): void {
  if (!obj) return;
  try {
    map.geoObjects.remove(obj);
  } catch {
    /* уже снят */
  }
}

function removeOverlayCircles(
  map: YMap,
  serviceRef: { current: YCircle | null },
  excludeRef: { current: Map<string, YCircle> },
): void {
  safeRemoveFromMap(map, serviceRef.current);
  serviceRef.current = null;
  for (const [, c] of excludeRef.current) {
    safeRemoveFromMap(map, c);
  }
  excludeRef.current.clear();
}

export type MasterTravelBaseMapProps = {
  apiKey: string | undefined;
  /** Сброс карты при обновлении профиля с сервера */
  mapVersion: string;
  cityHint: string;
  seedLat: number | null;
  seedLon: number | null;
  onPositionChange: (lat: number, lon: number) => void;
  /** Радиус зоны выезда от метки, км (0 — круг не рисуем). */
  travelRadiusKm: number;
  /** Зоны «сюда не выезжаю» (круги на карте). */
  excludeZones: MasterTravelExcludeZone[];
  /** Режим: следующий клик по карте добавит зону исключения (метку не двигаем). */
  excludePlacementActive?: boolean;
  onAddExclusionAt?: (lat: number, lon: number) => void;
  /** Только просмотр: без поиска, метка и зоны не редактируются (публичная карточка). */
  readOnly?: boolean;
};

export function MasterTravelBaseMap({
  apiKey,
  mapVersion,
  cityHint,
  seedLat,
  seedLon,
  onPositionChange,
  travelRadiusKm,
  excludeZones,
  excludePlacementActive = false,
  onAddExclusionAt,
  readOnly = false,
}: MasterTravelBaseMapProps) {
  const wrapRef = useRef<HTMLDivElement>(null);
  const mapInst = useRef<YMap | null>(null);
  const placemarkRef = useRef<YPlacemark | null>(null);
  const onPosRef = useRef(onPositionChange);
  const seedsRef = useRef({ seedLat, seedLon });
  const cityRef = useRef(cityHint);
  const radiusKmRef = useRef(travelRadiusKm);
  const excludeZonesRef = useRef(excludeZones);
  const excludePlacementRef = useRef(excludePlacementActive);
  const onAddExclusionRef = useRef(onAddExclusionAt);
  const readOnlyRef = useRef(readOnly);
  const syncOverlaysRef = useRef<(() => void) | null>(null);
  const serviceCircleRef = useRef<YCircle | null>(null);
  const excludeCirclesRef = useRef<Map<string, YCircle>>(new Map());

  const excludeZonesSignature = useMemo(
    () =>
      excludeZones
        .map((z) => `${z.id}:${z.latitude}:${z.longitude}:${z.radius_km}`)
        .join("|"),
    [excludeZones],
  );

  useLayoutEffect(() => {
    onPosRef.current = onPositionChange;
    seedsRef.current = { seedLat, seedLon };
    cityRef.current = cityHint;
    radiusKmRef.current = travelRadiusKm;
    excludeZonesRef.current = excludeZones;
    excludePlacementRef.current = excludePlacementActive;
    onAddExclusionRef.current = onAddExclusionAt;
    readOnlyRef.current = readOnly;
  }, [
    onPositionChange,
    seedLat,
    seedLon,
    cityHint,
    travelRadiusKm,
    excludeZones,
    excludePlacementActive,
    onAddExclusionAt,
    readOnly,
  ]);

  const [hint, setHint] = useState<"loading" | "error" | null>("loading");
  const [addressQuery, setAddressQuery] = useState("");
  const [geocodeBusy, setGeocodeBusy] = useState(false);
  const [geocodeMessage, setGeocodeMessage] = useState<string | null>(null);

  const runGeocodeSearch = useCallback(async () => {
    const raw = addressQuery.trim().replace(/\\/g, "/").replace(/\s+/g, " ");
    if (!raw) {
      setGeocodeMessage("Введите адрес или название места");
      return;
    }
    if (!placemarkRef.current || !mapInst.current) {
      setGeocodeMessage("Подождите, пока карта загрузится");
      return;
    }
    const city = cityRef.current.trim();
    const fullQuery =
      city && !raw.toLowerCase().includes(city.toLowerCase()) ? `${city}, ${raw}` : raw;

    setGeocodeMessage(null);
    setGeocodeBusy(true);
    try {
      const res = await fetch(`/api/yandex-geocode?q=${encodeURIComponent(fullQuery)}`);
      const data = (await res.json()) as {
        lat?: number;
        lon?: number;
        message?: string;
        error?: string;
      };
      if (!res.ok) {
        const msg =
          typeof data.message === "string" && data.message.trim()
            ? data.message
            : res.status === 404
              ? "Ничего не найдено — уточните запрос или поставьте метку вручную"
              : "Не удалось выполнить поиск. Проверьте ключ (нужен HTTP Геокодер) и сеть.";
        setGeocodeMessage(msg);
        return;
      }
      const lat = data.lat;
      const lon = data.lon;
      if (lat == null || lon == null || !Number.isFinite(lat) || !Number.isFinite(lon)) {
        setGeocodeMessage("Некорректный ответ геокодера");
        return;
      }
      placemarkRef.current.geometry.setCoordinates([lat, lon]);
      mapInst.current.setCenter([lat, lon], 16);
      onPosRef.current?.(lat, lon);
      syncOverlaysRef.current?.();
    } catch {
      setGeocodeMessage("Не удалось выполнить поиск. Проверьте сеть.");
    } finally {
      setGeocodeBusy(false);
    }
  }, [addressQuery]);

  useEffect(() => {
    if (!apiKey?.trim()) return;
    const root = wrapRef.current;
    if (!root) return;

    let cancelled = false;
    mapInst.current = null;
    placemarkRef.current = null;
    serviceCircleRef.current = null;
    excludeCirclesRef.current.clear();
    syncOverlaysRef.current = null;

    (async () => {
      try {
        await loadYandexMapsScript(apiKey.trim());
        if (cancelled || !wrapRef.current) return;
        const ymaps = window.ymaps!;
        const { seedLat: sl, seedLon: so } = seedsRef.current;

        let center: YCoord = [...DEFAULT_CENTER];
        if (sl != null && so != null && Number.isFinite(sl) && Number.isFinite(so)) {
          center = [sl, so];
        } else {
          const q = cityRef.current.trim() || "Москва";
          try {
            const res = await ymaps.geocode(q);
            const first = res.geoObjects.get(0);
            const c = first?.geometry?.getCoordinates();
            if (c && Array.isArray(c) && c.length >= 2) center = c;
          } catch {
            /* центр по умолчанию */
          }
        }

        const map = new ymaps.Map(root, {
          center,
          zoom: sl != null && so != null && Number.isFinite(sl) && Number.isFinite(so) ? 13 : 10,
          controls: ["zoomControl", "geolocationControl"],
        });

        const placemark = new ymaps.Placemark(
          center,
          readOnly
            ? {
                hintContent: "Точка отсчёта зоны выезда",
                balloonContent:
                  "Синий круг — зона выезда мастера (по прямой на карте). Красные круги — территории, куда выезд не осуществляется.",
              }
            : {
                hintContent: "Ваша метка на карте",
                balloonContent:
                  "Перетащите метку или щёлкните по карте — от неё считается расстояние в километрах. Синий круг — зона выезда; красные — куда не выезжаете.",
              },
          { draggable: !readOnly, preset: "islands#violetDotIcon", zIndex: 650 },
        );
        map.geoObjects.add(placemark);
        placemarkRef.current = placemark;
        mapInst.current = map;

        const syncOverlays = () => {
          const m = mapInst.current;
          const pm = placemarkRef.current;
          const y = window.ymaps;
          if (!m || !pm || !y) return;
          const coords = pm.geometry.getCoordinates();
          if (!coords || coords.length < 2) return;
          removeOverlayCircles(m, serviceCircleRef, excludeCirclesRef);
          const rKm = radiusKmRef.current;
          if (rKm > 0 && Number.isFinite(rKm)) {
            const circle = new y.Circle([coords, rKm * 1000], {}, SERVICE_ZONE_STYLE);
            m.geoObjects.add(circle);
            serviceCircleRef.current = circle;
          }
          let excludeZ = 410;
          for (const z of excludeZonesRef.current) {
            if (
              !Number.isFinite(z.latitude) ||
              !Number.isFinite(z.longitude) ||
              !Number.isFinite(z.radius_km) ||
              z.radius_km <= 0
            ) {
              continue;
            }
            const circ = new y.Circle([[z.latitude, z.longitude], z.radius_km * 1000], {}, {
              ...EXCLUDE_ZONE_BASE_STYLE,
              zIndex: excludeZ++,
            });
            m.geoObjects.add(circ);
            excludeCirclesRef.current.set(z.id, circ);
          }
        };

        syncOverlaysRef.current = syncOverlays;

        const emit = () => {
          const c = placemark.geometry.getCoordinates();
          if (c && c.length >= 2 && Number.isFinite(c[0]) && Number.isFinite(c[1])) {
            onPosRef.current?.(c[0], c[1]);
          }
          syncOverlays();
        };
        emit();
        placemark.events.add("dragend", emit);
        map.events.add("click", (e) => {
          if (readOnlyRef.current) return;
          const coords = e.get("coords");
          if (!coords || coords.length < 2) return;
          if (excludePlacementRef.current && onAddExclusionRef.current) {
            onAddExclusionRef.current(coords[0], coords[1]);
            return;
          }
          placemark.geometry.setCoordinates(coords);
          onPosRef.current?.(coords[0], coords[1]);
          syncOverlays();
        });

        if (!cancelled) setHint(null);
      } catch {
        if (!cancelled) setHint("error");
      }
    })();

    return () => {
      cancelled = true;
      syncOverlaysRef.current = null;
      serviceCircleRef.current = null;
      excludeCirclesRef.current = new Map();
      try {
        mapInst.current?.destroy();
      } catch {
        /* ignore */
      }
      mapInst.current = null;
      placemarkRef.current = null;
    };
  }, [apiKey, mapVersion, readOnly]);

  useEffect(() => {
    if (hint !== null) return;
    syncOverlaysRef.current?.();
  }, [hint, travelRadiusKm, excludeZonesSignature]);

  if (!apiKey?.trim()) {
    return (
      <div className="rounded-lg border border-amber-200 bg-amber-50/60 px-3 py-3 text-sm text-amber-950">
        Для карты задайте ключ API в переменной окружения{" "}
        <code className="rounded bg-amber-100/80 px-1">NEXT_PUBLIC_YANDEX_MAPS_API_KEY</code> (кабинет
        разработчика Яндекс.Карт → JavaScript API и HTTP Геокодер).
      </div>
    );
  }

  if (hint === "error") {
    return (
      <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-3 text-sm text-destructive">
        Не удалось загрузить Яндекс.Карты. Проверьте ключ и сеть.
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {!readOnly ? (
        <div className="space-y-2 rounded-lg border border-border bg-background/80 p-3">
          <Label htmlFor="travel-base-address-search">Поиск по адресу</Label>
          <p className="text-xs text-muted-foreground">
            Город, улица, дом или название организации — карта перенесёт метку к найденной точке.
          </p>
          <div className="flex flex-col gap-2 sm:flex-row">
            <Input
              id="travel-base-address-search"
              value={addressQuery}
              onChange={(e) => {
                setAddressQuery(e.target.value);
                if (geocodeMessage) setGeocodeMessage(null);
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  void runGeocodeSearch();
                }
              }}
              placeholder="Например: Санкт-Петербург, Невский проспект, 28"
              disabled={hint === "loading" || geocodeBusy}
              autoComplete="street-address"
            />
            <Button
              type="button"
              variant="secondary"
              className="shrink-0 sm:w-36"
              disabled={hint === "loading" || geocodeBusy}
              onClick={() => void runGeocodeSearch()}
            >
              {geocodeBusy ? "Поиск…" : "Найти"}
            </Button>
          </div>
          {geocodeMessage ? (
            <p className="text-xs text-amber-800 dark:text-amber-200" role="status">
              {geocodeMessage}
            </p>
          ) : null}
        </div>
      ) : null}
      <div className="relative h-[min(360px,55vh)] w-full min-h-[240px] overflow-hidden rounded-lg border border-border bg-muted/30">
        <div ref={wrapRef} className="h-full w-full" />
        {hint === "loading" ? (
          <div className="pointer-events-none absolute inset-0 flex items-center justify-center bg-muted/40 text-sm text-muted-foreground">
            Загрузка карты…
          </div>
        ) : null}
      </div>
      <p className="text-xs text-muted-foreground">
        {readOnly
          ? "Радиус указан по прямой на карте; фактический путь по дорогам может быть дольше. Уточняйте выезд у мастера."
          : "Синий круг — зона выезда от метки (поле «километров» ниже). Красные — места, куда не выезжаете, даже внутри синей зоны. Поиск по адресу, перетаскивание метки или обычный клик по карте двигают метку; режим «добавить исключение» задаётся кнопкой под списком зон."}
      </p>
    </div>
  );
}
