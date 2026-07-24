import type { CapacitorConfig } from "@capacitor/cli";

// ponytail: remote-URL подход. Приложение — тонкая нативная оболочка над
// задеплоенным SSR-сайтом. Так вся серверная архитектура (proxy.ts CSP-nonce,
// rewrites, Server Components) работает как есть, без статического export.
// Риск Apple 4.2 закрывается нативными плагинами (пуши/шаринг/камера).
// server.url переопределяется env-переменной CAP_SERVER_URL — для локальной
// разработки (напр. CAP_SERVER_URL=http://localhost:3000 против докера).
// Без неё берётся прод-placeholder. cleartext (http) включается сам только
// для локального http-адреса; в проде остаётся https.
const serverUrl = process.env.CAP_SERVER_URL ?? "https://example.com"; // ponytail: set real prod domain before store submit
const isLocalHttp = serverUrl.startsWith("http://");
// Хост server.url — держим навигацию по нему ВНУТРИ приложения (см. allowNavigation
// ниже). Без этого Capacitor открывает клики по ссылкам в системном Safari.
const serverHost = (() => {
  try {
    return new URL(serverUrl).hostname;
  } catch {
    return "";
  }
})();

const config: CapacitorConfig = {
  // ВАЖНО: appId = permanent bundle ID в App Store / Google Play, поменять
  // после публикации нельзя. Замени на реальный reverse-domain бренда.
  appId: "io.banya.app", // ponytail: placeholder, set real bundle ID before store submit
  appName: "Banya",

  // Локальный fallback (index.html из mobile/www) — грузится, только если
  // server.url недоступен. Основной контент идёт с server.url ниже.
  webDir: "mobile/www",

  server: {
    url: serverUrl,
    cleartext: isLocalHttp, // http разрешаем ТОЛЬКО для локальной отладки
    // Клики по ссылкам своего хоста остаются в WebView, а не улетают в Safari.
    allowNavigation: serverHost ? [serverHost] : undefined,
  },
};

export default config;
