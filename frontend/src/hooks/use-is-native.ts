import * as React from 'react'
import { Capacitor } from '@capacitor/core'

// True only inside the Capacitor native shell (iOS/Android app), false on web.
// Starts false so the SSR markup and the first client render match (no hydration
// mismatch); flips after mount once the native bridge is readable. The footer it
// gates is below the fold, so the one-frame flash before the effect runs is unseen.
export function useIsNative() {
  const [isNative, setIsNative] = React.useState(false)

  React.useEffect(() => {
    setIsNative(Capacitor.isNativePlatform())
  }, [])

  return isNative
}
