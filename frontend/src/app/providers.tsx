"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Suspense, useEffect, useState } from "react";
import { useAuthStore } from "@/store/auth";
import { AnalyticsPageView } from "./analytics-pageview";

function AuthHydrator({ children }: { children: React.ReactNode }) {
  useEffect(() => {
    // bootstrap() мигрирует legacy-сессию, делает тихий refresh через
    // httpOnly cookie и восстанавливает профиль — чтобы перезагрузка страницы
    // не выкидывала залогиненного пользователя.
    void useAuthStore.getState().bootstrap();
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
