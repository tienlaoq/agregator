const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

/**
 * Sends a lightweight product event to api-gateway (structured logs + optional NATS).
 * Event names: lowercase letters, digits, underscore; must start with a letter.
 */
export function track(name: string, props?: Record<string, unknown>): void {
  if (typeof window === "undefined") return;
  const payload = JSON.stringify({ name, props: props ?? {} });
  const url = `${API_URL}/api/v1/analytics/events`;
  try {
    if (typeof navigator !== "undefined" && navigator.sendBeacon) {
      const blob = new Blob([payload], { type: "application/json" });
      if (navigator.sendBeacon(url, blob)) return;
    }
  } catch {
    // fall through to fetch
  }
  void fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: payload,
    keepalive: true,
    mode: "cors",
    credentials: "omit",
  }).catch(() => {});
}
