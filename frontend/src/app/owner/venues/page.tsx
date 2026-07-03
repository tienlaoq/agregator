"use client";

import { useEffect } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Building2, Plus } from "lucide-react";
import { redirectToLogin } from "@/lib/auth-redirect";
import { getOwnerVenues } from "@/lib/api";
import { useAuthStore } from "@/store/auth";

export default function OwnerVenuesPage() {
  const router = useRouter();
  const { token, user, hydrated } = useAuthStore();
  const canOwnerCabinet =
    user?.role === "venue_owner" ||
    user?.role === "master" ||
    user?.role === "user";
  const canCreateVenue =
    user?.role === "venue_owner" || user?.role === "master";

  useEffect(() => {
    if (hydrated && (!token || !canOwnerCabinet)) redirectToLogin();
  }, [hydrated, token, canOwnerCabinet]);

  const { data: venues, isLoading } = useQuery({
    queryKey: ["owner-venues"],
    queryFn: getOwnerVenues,
    enabled: !!token && canOwnerCabinet,
  });

  // Any venue → straight into its workspace; switching and creating live in the
  // sidebar. The old owner dashboard is fully retired.
  useEffect(() => {
    if (venues && venues.length > 0) {
      router.replace(`/owner/venues/${venues[0].id}`);
    }
  }, [venues, router]);

  if (!hydrated || !token) return null;

  // Loading, or the redirect into a workspace is in flight → spinner.
  if (isLoading || !venues || venues.length > 0) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-muted border-t-primary" />
      </div>
    );
  }

  // No venues yet → clean empty state.
  return (
    <div className="container mx-auto max-w-2xl px-4 py-10">
      <h1 className="mb-6 text-2xl font-bold text-foreground md:text-3xl">
        Мои заведения
      </h1>
      <Card className="border-dashed">
        <CardContent className="flex flex-col items-center gap-3 py-16 text-center">
          <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-muted text-muted-foreground">
            <Building2 className="h-6 w-6" />
          </div>
          <p className="max-w-sm text-sm text-muted-foreground">
            У вас пока нет заведений. Добавьте первое — откроется кабинет с
            расписанием, бронями и гостями.
          </p>
          {canCreateVenue ? (
            <Button asChild className="mt-1">
              <Link href="/owner/venues/new">
                <Plus className="mr-1 h-4 w-4" />
                Добавить заведение
              </Link>
            </Button>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}
