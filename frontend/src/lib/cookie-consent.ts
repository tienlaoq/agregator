// Cookie/analytics consent state, shared between the banner and analytics.
// Persisted in localStorage; a custom event lets listeners (Yandex Metrika)
// react to a decision immediately, without a page reload.

export type ConsentValue = "accepted" | "rejected"

const STORAGE_KEY = "banyagid:cookie-consent"
const EVENT = "banyagid:cookie-consent-change"

/** Current decision, or null if the user hasn't chosen yet. SSR-safe. */
export function getConsent(): ConsentValue | null {
  if (typeof window === "undefined") return null
  const v = window.localStorage.getItem(STORAGE_KEY)
  return v === "accepted" || v === "rejected" ? v : null
}

/** Persist a decision and notify listeners in the same tab. */
export function setConsent(value: ConsentValue): void {
  if (typeof window === "undefined") return
  window.localStorage.setItem(STORAGE_KEY, value)
  window.dispatchEvent(new CustomEvent<ConsentValue>(EVENT, { detail: value }))
}

/** Subscribe to consent changes (same-tab event + cross-tab storage). */
export function onConsentChange(listener: (value: ConsentValue | null) => void): () => void {
  if (typeof window === "undefined") return () => {}
  const handleEvent = () => listener(getConsent())
  const handleStorage = (e: StorageEvent) => {
    if (e.key === STORAGE_KEY) listener(getConsent())
  }
  window.addEventListener(EVENT, handleEvent)
  window.addEventListener("storage", handleStorage)
  return () => {
    window.removeEventListener(EVENT, handleEvent)
    window.removeEventListener("storage", handleStorage)
  }
}
