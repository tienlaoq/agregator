import * as React from "react"

import { getConsent, onConsentChange, type ConsentValue } from "@/lib/cookie-consent"

/**
 * Reactive consent state. Returns `undefined` until mounted (avoids hydration
 * mismatch — the server can't know localStorage), then the stored decision or
 * `null` if the user hasn't chosen yet.
 */
export function useCookieConsent(): ConsentValue | null | undefined {
  const [consent, setConsentState] = React.useState<ConsentValue | null | undefined>(undefined)

  React.useEffect(() => {
    setConsentState(getConsent())
    return onConsentChange(setConsentState)
  }, [])

  return consent
}
