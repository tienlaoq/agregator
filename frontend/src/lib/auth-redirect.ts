/** Full-page redirect avoids App Router soft-navigation fetch races (e.g. “Failed to fetch”). */
export function redirectToLogin(): void {
  if (typeof window === "undefined") return;
  if (window.location.pathname.startsWith("/auth/login")) return;
  window.location.assign("/auth/login");
}
