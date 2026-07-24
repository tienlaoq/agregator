"use client";

import { useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import { Capacitor } from "@capacitor/core";
import { useAuthStore } from "@/store/auth";
import { registerPushDevice, unregisterPushDevice } from "@/lib/api";

// Data map delivered inside an FCM push (built by notification-service's fcm
// sender): { type, notification_id, payload } where payload is the raw JSON the
// bell also stores. Everything is a string — FCM data values are strings only.
type PushData = { type?: string; notification_id?: string; payload?: string };

/**
 * Maps a tapped push to an in-app route. The web bell itself does not deep-link
 * per type, so this stays deliberately coarse: master notifications open the
 * master cabinet, venue-scoped ones open that venue, everything else the home
 * screen. ponytail: coarse routing; refine per-type when the bell does too.
 */
function pushTargetPath(data: PushData | undefined): string {
  if (!data) return "/";
  let payload: Record<string, unknown> = {};
  try {
    if (data.payload) payload = JSON.parse(data.payload) as Record<string, unknown>;
  } catch {
    // malformed payload → fall through to defaults
  }
  const kind = String(payload.kind ?? data.type ?? "");
  if (kind.startsWith("master")) return "/owner/master";
  const venueId = payload.venue_id;
  if (typeof venueId === "string" && venueId) return `/owner/venues/${venueId}`;
  return "/";
}

/**
 * PushRegistration wires Capacitor/FCM push into the app. It renders nothing and
 * runs only inside the native shell for an authenticated user:
 *  - requests notification permission, then registers the device FCM token with
 *    the backend (which delivers pushes when the app is backgrounded);
 *  - re-registers when FCM rotates the token;
 *  - routes taps to the relevant screen;
 *  - unregisters the token on logout.
 * On the web it is inert (the WebSocket bell handles realtime there).
 */
export function PushRegistration() {
  const token = useAuthStore((s) => s.token);
  const router = useRouter();
  // Last FCM token we registered, so logout can unregister exactly it.
  const registeredToken = useRef<string | null>(null);
  // Keep the latest router in a ref: plugin listeners fire outside React render.
  const routerRef = useRef(router);
  routerRef.current = router;

  useEffect(() => {
    if (!Capacitor.isNativePlatform() || !token) return;

    let active = true;
    const removers: Array<() => void> = [];

    (async () => {
      const { FirebaseMessaging } = await import("@capacitor-firebase/messaging");

      let perm = await FirebaseMessaging.checkPermissions();
      if (perm.receive === "prompt") {
        perm = await FirebaseMessaging.requestPermissions();
      }
      if (!active || perm.receive !== "granted") return;

      const platform = Capacitor.getPlatform();

      const sendToken = async (value: string) => {
        if (!value) return;
        try {
          await registerPushDevice(value, platform);
          registeredToken.current = value;
        } catch {
          // best-effort: a failed registration just means no push until retry
        }
      };

      try {
        const { token: fcmToken } = await FirebaseMessaging.getToken();
        if (active) await sendToken(fcmToken);
      } catch {
        // getToken can fail before APNs/FCM is ready; tokenReceived covers it
      }

      const tokenSub = await FirebaseMessaging.addListener(
        "tokenReceived",
        ({ token: fcmToken }) => void sendToken(fcmToken),
      );
      removers.push(() => void tokenSub.remove());

      const tapSub = await FirebaseMessaging.addListener(
        "notificationActionPerformed",
        (event) => {
          const data = event.notification?.data as PushData | undefined;
          routerRef.current.push(pushTargetPath(data));
        },
      );
      removers.push(() => void tapSub.remove());
    })();

    return () => {
      active = false;
      for (const remove of removers) remove();
    };
  }, [token]);

  // On logout (token cleared) drop the server-side token so a signed-out device
  // stops receiving this user's pushes. Runs after the effect above tears down.
  const prevToken = useRef<string | null>(token);
  useEffect(() => {
    if (prevToken.current && !token && registeredToken.current) {
      void unregisterPushDevice(registeredToken.current).catch(() => {});
      registeredToken.current = null;
    }
    prevToken.current = token;
  }, [token]);

  return null;
}
