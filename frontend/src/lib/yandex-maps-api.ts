/**
 * Минимальные типы и загрузчик скрипта Яндекс.Карт JS API 2.1.
 * @see https://yandex.ru/dev/jsapi-v2-1/doc/ru/
 */

export type YCoord = number[];

export type YPlacemark = {
  geometry: { getCoordinates: () => YCoord; setCoordinates: (c: YCoord) => void };
  events: { add: (ev: string, fn: () => void) => void };
};

export type YCircle = {
  geometry: { getCoordinates: () => YCoord };
};

export type YMap = {
  destroy: () => void;
  geoObjects: {
    add: (o: YPlacemark | YCircle) => void;
    remove: (o: YPlacemark | YCircle) => void;
  };
  setCenter: (c: YCoord, zoom?: number, opts?: Record<string, unknown>) => void;
  events: { add: (ev: string, fn: (e: { get: (k: string) => YCoord }) => void) => void };
};

export type YGeocodeResult = {
  geoObjects: {
    get: (i: number) => { geometry: { getCoordinates: () => YCoord } } | undefined;
  };
};

export type YMapsNS = {
  ready: (fn: () => void) => void;
  Map: new (el: HTMLElement, state: Record<string, unknown>) => YMap;
  Placemark: new (
    coords: YCoord,
    props?: Record<string, unknown>,
    opts?: Record<string, unknown>,
  ) => YPlacemark;
  Circle: new (
    geometry: [YCoord, number],
    properties?: Record<string, unknown>,
    options?: Record<string, unknown>,
  ) => YCircle;
  geocode: (q: string) => Promise<YGeocodeResult>;
};

declare global {
  interface Window {
    ymaps?: YMapsNS;
  }
}

const SCRIPT_SELECTOR = "script[data-yandex-maps-loader]";

/** Подключает api-maps.yandex.ru/2.1 один раз на документ; idempotent. */
export function loadYandexMapsScript(apiKey: string): Promise<void> {
  const key = apiKey.trim();
  return new Promise((resolve, reject) => {
    if (typeof window === "undefined") {
      reject(new Error("no window"));
      return;
    }
    const ready = () => {
      window.ymaps!.ready(() => resolve());
    };
    if (window.ymaps) {
      ready();
      return;
    }
    const existing = document.querySelector<HTMLScriptElement>(SCRIPT_SELECTOR);
    if (existing) {
      existing.addEventListener(
        "load",
        () => {
          if (window.ymaps) window.ymaps.ready(() => resolve());
          else reject(new Error("ymaps script"));
        },
        { once: true },
      );
      existing.addEventListener("error", () => reject(new Error("ymaps script")), {
        once: true,
      });
      return;
    }
    const s = document.createElement("script");
    s.dataset.yandexMapsLoader = "1";
    s.async = true;
    s.src = `https://api-maps.yandex.ru/2.1/?apikey=${encodeURIComponent(key)}&lang=ru_RU`;
    s.onload = () => ready();
    s.onerror = () => reject(new Error("ymaps script"));
    document.head.appendChild(s);
  });
}
