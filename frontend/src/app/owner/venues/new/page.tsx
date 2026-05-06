"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { createVenue } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import { getRawPhone } from "@/components/banya/phone-input";
import { PartnerVenueRegistrationCard } from "@/components/banya/partner-venue-registration-card";
import {
  buildPartnerStyleCreateVenueRequest,
  emptyPartnerVenueForm,
  partnerVenueStepCanProceed,
  type PartnerVenueFormValues,
} from "@/lib/partner-venue";

export default function CreateVenuePage() {
  const router = useRouter();
  const { token, user, hydrated } = useAuthStore();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [venueFields, setVenueFields] = useState<PartnerVenueFormValues>(() =>
    emptyPartnerVenueForm(),
  );
  const [venuePhone, setVenuePhone] = useState("");
  const phoneSeededRef = useRef(false);
  const submittingRef = useRef(false);

  const patchVenue = (p: Partial<PartnerVenueFormValues>) =>
    setVenueFields((prev) => ({ ...prev, ...p }));

  const canCreateVenue =
    user?.role === "venue_owner" || user?.role === "master";

  useEffect(() => {
    if (hydrated && (!token || !canCreateVenue)) {
      router.push("/auth/login");
    }
  }, [hydrated, token, user, router, canCreateVenue]);

  useEffect(() => {
    if (!hydrated || phoneSeededRef.current || !user?.phone?.trim()) return;
    setVenuePhone(user.phone.trim());
    phoneSeededRef.current = true;
  }, [hydrated, user?.phone]);

  const rawVenuePhone = getRawPhone(venuePhone).trim();
  const canSubmit = partnerVenueStepCanProceed(venueFields) && rawVenuePhone.length > 0;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (submittingRef.current || !canSubmit) return;
    submittingRef.current = true;
    setError("");
    setLoading(true);
    try {
      await createVenue(
        buildPartnerStyleCreateVenueRequest(venueFields, rawVenuePhone),
      );
      router.push("/owner/venues");
    } catch {
      setError("Не удалось создать заведение. Попробуйте позже.");
    } finally {
      setLoading(false);
      submittingRef.current = false;
    }
  };

  if (!hydrated || !token) return null;

  return (
    <div className="container mx-auto px-4 py-8">
      <h1 className="mb-2 text-2xl font-bold text-foreground">Добавить заведение</h1>
      <p className="mb-6 text-sm text-muted-foreground">
        Та же форма, что при регистрации партнёра: черновик создаётся сразу, модератор увидит
        карточку после отправки на проверку из редактирования.
      </p>
      {error ? (
        <p className="mb-4 text-sm text-destructive">{error}</p>
      ) : null}

      <form onSubmit={handleSubmit}>
        <PartnerVenueRegistrationCard
          values={venueFields}
          onChange={patchVenue}
          showVenuePhone
          venuePhone={venuePhone}
          onVenuePhoneChange={setVenuePhone}
          footer={
            <Button type="submit" className="mt-2 w-full gap-2" size="lg" disabled={loading || !canSubmit}>
              {loading ? "Создание..." : "Создать заведение"}
            </Button>
          }
        />
      </form>
    </div>
  );
}
