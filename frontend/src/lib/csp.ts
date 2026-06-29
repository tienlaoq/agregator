// Content-Security-Policy builder, shared by the proxy (which injects a
// per-request nonce). script-src uses nonce + 'strict-dynamic': modern browsers
// trust only the nonce'd bootstrap and whatever it loads (Next chunks, and our
// dynamically-injected Яндекс.Карты/Метрика scripts), ignoring host allowlists
// and 'unsafe-inline'. The trailing 'unsafe-inline' https: are a fallback for
// browsers that don't support 'strict-dynamic'. style-src keeps 'unsafe-inline'
// because Tailwind/runtime styles need it (lower risk than scripts).
//
// 'strict-dynamic' covers loadYandexMapsScript() (createElement from trusted app
// code) and the Метрика inline init (which carries the nonce, see
// components/seo/yandex-metrika.tsx). Non-script directives keep explicit host
// allowlists since 'strict-dynamic' only affects script-src.

const mediaHost = process.env.NEXT_PUBLIC_MEDIA_HOST ?? ""

export function buildCsp(nonce: string, isDev: boolean): string {
  const directives: Record<string, string | undefined> = {
    "default-src": "'self'",
    "script-src": [
      "'self'",
      `'nonce-${nonce}'`,
      "'strict-dynamic'",
      isDev ? "'unsafe-eval'" : "",
      // Fallback for browsers without 'strict-dynamic' (ignored by modern ones).
      "'unsafe-inline'",
      "https:",
      isDev ? "http:" : "",
    ]
      .filter(Boolean)
      .join(" "),
    "style-src": "'self' 'unsafe-inline'", // Tailwind/CSS-in-JS require inline
    "img-src": [
      "'self'",
      "data:",
      "blob:",
      "https://images.unsplash.com",
      "https://mc.yandex.ru",
      "https://mc.yandex.com",
      // Яндекс.Карты тайлы
      "https://*.maps.yandex.net",
      "https://*.yandex.net",
      mediaHost,
      isDev ? "http://localhost:*" : "",
    ]
      .filter(Boolean)
      .join(" "),
    "font-src": "'self' https://fonts.gstatic.com data:",
    "connect-src": [
      "'self'",
      process.env.NEXT_PUBLIC_API_URL ?? "",
      "https://api-maps.yandex.ru",
      "https://geocode-maps.yandex.ru",
      "https://suggest-maps.yandex.ru",
      "https://mc.yandex.ru",
      "https://mc.yandex.com",
      process.env.NEXT_PUBLIC_WS_URL ?? "",
      isDev ? "ws://localhost:* http://localhost:*" : "",
    ]
      .filter(Boolean)
      .join(" "),
    "frame-src": "'none'",
    "object-src": "'none'",
    "base-uri": "'self'",
    "form-action": "'self'",
    "frame-ancestors": "'none'",
    "upgrade-insecure-requests": isDev ? undefined : "",
  }

  return Object.entries(directives)
    .filter(([, v]) => v !== undefined)
    .map(([k, v]) => (v ? `${k} ${v}` : k))
    .join("; ")
}
