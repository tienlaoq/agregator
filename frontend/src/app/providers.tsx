"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Suspense, useEffect, useState } from "react";
import { useAuthStore } from "@/store/auth";
import { AnalyticsPageView } from "./analytics-pageview";

function AuthHydrator({ children }: { children: React.ReactNode }) {
  useEffect(() => {
    useAuthStore.getState().hydrate();
  }, []);

  return children;
}

export function Providers({ children }: { children: React.ReactNode }) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            staleTime: 60 * 1000,
            retry: 1,
          },
        },
      }),
  );

  return (
    <QueryClientProvider client={queryClient}>
      <Suspense fallback={null}>
        <AnalyticsPageView />
      </Suspense>
      <AuthHydrator>{children}</AuthHydrator>
    </QueryClientProvider>
  );
}
