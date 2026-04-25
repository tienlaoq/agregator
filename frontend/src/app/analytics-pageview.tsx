"use client";

import { usePathname, useSearchParams } from "next/navigation";
import { useEffect, useRef } from "react";
import { track } from "@/lib/analytics";

/** Emits `page_view` on route changes (requires Suspense boundary for useSearchParams). */
export function AnalyticsPageView() {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const prev = useRef<string | null>(null);

  useEffect(() => {
    const qs = searchParams?.toString();
    const full = qs ? `${pathname}?${qs}` : pathname ?? "/";
    if (prev.current === full) return;
    prev.current = full;
    track("page_view", { path: full });
  }, [pathname, searchParams]);

  return null;
}
