"use client";

import { useEffect, useMemo } from "react";
import { useRouter } from "next/navigation";
import { useQueries, useQuery } from "@tanstack/react-query";
import {
  OwnerDashboardSection,
  bookingsTodayByVenue,
  computeOwnerDashboardStats,
  mergeOwnerVenueBookings,
} from "@/components/banya/owner-dashboard-section";
import { getOwnerVenueBookings, getOwnerVenues } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
export default function OwnerVenuesPage() {
  const router = useRouter();
  const { token, user, hydrated } = useAuthStore();
  const isVenueOwner = user?.role === "venue_owner";

  useEffect(() => {
    if (hydrated && (!token || !isVenueOwner)) {
      router.push("/auth/login");
    }
  }, [hydrated, token, user, router, isVenueOwner]);

  const { data: venues, isLoading: isLoadingVenues } = useQuery({
    queryKey: ["owner-venues"],
    queryFn: getOwnerVenues,
    enabled: !!token && isVenueOwner,
  });

  const venueIds = useMemo(() => venues?.map((v) => v.id) ?? [], [venues]);

  const bookingQueries = useQueries({
    queries: venueIds.map((id) => ({
      queryKey: ["owner-venue-bookings", id],
      queryFn: () => getOwnerVenueBookings(id, { page_size: 50 }),
      enabled: !!token && isVenueOwner && venueIds.length > 0,
    })),
  });

  const allBookings = mergeOwnerVenueBookings(bookingQueries);
  const stats = computeOwnerDashboardStats(venues, allBookings);
  const todayBookingsByVenueId = bookingsTodayByVenue(venues, allBookings);
  const recentBookings = allBookings.slice(0, 8);

  const isLoadingBookings =
    venueIds.length > 0 && bookingQueries.some((q) => q.isPending);

  if (!hydrated || !token) return null;

  return (
    <OwnerDashboardSection
      venues={venues}
      isLoadingVenues={isLoadingVenues}
      isLoadingBookings={isLoadingBookings}
      stats={stats}
      todayBookingsByVenueId={todayBookingsByVenueId}
      recentBookings={recentBookings}
      onAddVenue={() => router.push("/owner/venues/new")}
    />
  );
}
