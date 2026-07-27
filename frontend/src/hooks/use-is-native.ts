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

// Inverse of useIsNative for content that must NEVER appear in the app, even for
// one frame: starts false, so the gated block is hidden until the bridge is read
// and only then appears on web. useIsNative starts false too, which means a
// !isNative gate would flash the block inside the app before the effect runs —
// fine for a footer, not for the VK/Яндекс buttons that App Review must not see
// (guideline 4.8 demands Sign in with Apple next to any social login, and
// Russian law forbids offering it).
export function useIsWeb() {
  const [isWeb, setIsWeb] = React.useState(false)

  React.useEffect(() => {
    setIsWeb(!Capacitor.isNativePlatform())
  }, [])

  return isWeb
}
