"use client";

import { useEffect, useState, useRef } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { createVenue } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import {
  DEFAULT_VENUE_SOCIAL_LINKS,
  VENUE_TYPE_LABELS,
  trimVenueSocialLinks,
  venueServiceLinesForApi,
  type CreateVenueFormState,
} from "@/lib/types";
import { VenueSocialLinksFields } from "@/components/banya/venue-social-links-fields";
import { VenueServicesFields } from "@/components/banya/venue-services-fields";
import { CityCombobox } from "@/components/banya/city-combobox";
import { AddressSuggest } from "@/components/banya/address-suggest";
import { PhoneInput, getRawPhone } from "@/components/banya/phone-input";
import { Plus, X } from "lucide-react";

export default function CreateVenuePage() {
  const router = useRouter();
  const { token, user, hydrated } = useAuthStore();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const [form, setForm] = useState<CreateVenueFormState>({
    name: "",
    type: "banya",
    address: "",
    city: "",
    description: "",
    phone: "",
    price_from: 0,
    capacity: 10,
    amenities: [],
    services: [],
    legal_entity_name: "",
    inn: "",
    ogrn: "",
    public_listing_url: "",
    verification_note: "",
    social_links: { ...DEFAULT_VENUE_SOCIAL_LINKS },
  });

  const [newAmenity, setNewAmenity] = useState("");
  const canCreateVenue =
    user?.role === "venue_owner" || user?.role === "master";

  useEffect(() => {
    if (hydrated && (!token || !canCreateVenue)) {
      router.push("/auth/login");
    }
  }, [hydrated, token, user, router, canCreateVenue]);

  const updateField = <K extends keyof CreateVenueFormState>(
    key: K,
    value: CreateVenueFormState[K],
  ) => {
    setForm((prev) => ({ ...prev, [key]: value }));
  };

  const addAmenity = () => {
    if (newAmenity.trim()) {
      updateField("amenities", [...form.amenities, newAmenity.trim()]);
      setNewAmenity("");
    }
  };

  const removeAmenity = (index: number) => {
    updateField(
      "amenities",
      form.amenities.filter((_, i) => i !== index),
    );
  };

  const submittingRef = useRef(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (submittingRef.current) return;
    submittingRef.current = true;
    setError("");
    setLoading(true);
    try {
      await createVenue({
        ...form,
        phone: getRawPhone(form.phone),
        inn: form.inn.replace(/\D/g, ""),
        ogrn: form.ogrn.replace(/\D/g, ""),
        verification_note: form.verification_note?.trim() || undefined,
        social_links: trimVenueSocialLinks(form.social_links ?? DEFAULT_VENUE_SOCIAL_LINKS),
        services: venueServiceLinesForApi(form.services),
        price_from: 0,
        amenities: [],
        halls: [
          {
            name: "Основной зал",
            price_from: form.price_from,
            capacity: form.capacity,
            amenities: form.amenities,
            sort_order: 0,
          },
        ],
      });
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
    <div className="mx-auto max-w-2xl px-4 py-8">
      <h1 className="mb-6 text-2xl font-bold">Добавить заведение</h1>

      <form onSubmit={handleSubmit} className="space-y-6">
        {/* Basic info */}
        <Card>
          <CardHeader>
            <CardTitle>Основная информация</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="name">Название</Label>
              <Input
                id="name"
                placeholder="Русская баня на Пресне"
                value={form.name}
                onChange={(e) => updateField("name", e.target.value)}
                required
              />
            </div>

            <div className="space-y-2">
              <Label>Тип заведения</Label>
              <div className="grid grid-cols-3 gap-2">
                {Object.entries(VENUE_TYPE_LABELS).map(([value, label]) => (
                  <button
                    key={value}
                    type="button"
                    onClick={() =>
                      updateField("type", value as CreateVenueFormState["type"])
                    }
                    className={`rounded-lg border p-2.5 text-center text-sm transition-colors ${
                      form.type === value
                        ? "border-primary bg-primary/5 font-medium text-primary"
                        : "border-border text-muted-foreground hover:bg-muted"
                    }`}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </div>

            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label>Город</Label>
                <CityCombobox
                  value={form.city}
                  onChange={(v) => updateField("city", v)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="phone">Телефон</Label>
                <PhoneInput
                  id="phone"
                  value={form.phone}
                  onChange={(v) => updateField("phone", v)}
                  required
                />
              </div>
            </div>

            <div className="space-y-2">
              <Label>Адрес</Label>
              <AddressSuggest
                value={form.address}
                onChange={(v) => updateField("address", v)}
                city={form.city}
                placeholder="ул. Пример, д. 1"
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="description">Описание</Label>
              <Textarea
                id="description"
                placeholder="Расскажите о вашем заведении..."
                value={form.description}
                onChange={(e) => updateField("description", e.target.value)}
                required
              />
            </div>

            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="price">Цена зала (₽/час)</Label>
                <Input
                  id="price"
                  type="number"
                  min={0}
                  placeholder="1500"
                  value={form.price_from || ""}
                  onChange={(e) =>
                    updateField("price_from", Number(e.target.value))
                  }
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="capacity">Вместимость зала (чел.)</Label>
                <Input
                  id="capacity"
                  type="number"
                  min={1}
                  value={form.capacity || ""}
                  onChange={(e) =>
                    updateField("capacity", Number(e.target.value))
                  }
                  required
                />
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Соцсети и мессенджеры</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <p className="text-sm text-muted-foreground">
              По желанию — ссылки для гостей (ВК, Telegram, MAX и др.).
            </p>
            <VenueSocialLinksFields
              value={form.social_links ?? DEFAULT_VENUE_SOCIAL_LINKS}
              onChange={(next) => updateField("social_links", next)}
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Проверка владельца</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-sm text-muted-foreground">
              Укажите данные как в ЕГРЮЛ/ЕГРИП и ссылку на публичную карточку заведения (Яндекс.Карты, 2ГИС). Модератор сверит их с открытыми источниками.
            </p>
            <div className="space-y-2">
              <Label htmlFor="legal_entity_name">Наименование ИП / организации</Label>
              <Input
                id="legal_entity_name"
                value={form.legal_entity_name}
                onChange={(e) => updateField("legal_entity_name", e.target.value)}
                required
              />
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="inn">ИНН</Label>
                <Input
                  id="inn"
                  inputMode="numeric"
                  value={form.inn}
                  onChange={(e) => updateField("inn", e.target.value)}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="ogrn">ОГРН / ОГРНИП</Label>
                <Input
                  id="ogrn"
                  inputMode="numeric"
                  value={form.ogrn}
                  onChange={(e) => updateField("ogrn", e.target.value)}
                  required
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="public_listing_url">Ссылка на карточку на картах</Label>
              <Input
                id="public_listing_url"
                type="url"
                placeholder="https://..."
                value={form.public_listing_url}
                onChange={(e) => updateField("public_listing_url", e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="verification_note">Комментарий для модерации (необязательно)</Label>
              <Textarea
                id="verification_note"
                value={form.verification_note ?? ""}
                onChange={(e) => updateField("verification_note", e.target.value)}
                rows={2}
              />
            </div>
          </CardContent>
        </Card>

        {/* Amenities */}
        <Card>
          <CardHeader>
            <CardTitle>Удобства в зале</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="flex flex-wrap gap-2">
              {form.amenities.map((a, i) => (
                <span
                  key={i}
                  className="inline-flex items-center gap-1 rounded-full bg-primary/10 px-3 py-1 text-sm text-primary"
                >
                  {a}
                  <button
                    type="button"
                    onClick={() => removeAmenity(i)}
                    className="ml-0.5 hover:text-destructive"
                  >
                    <X className="h-3 w-3" />
                  </button>
                </span>
              ))}
            </div>
            <div className="flex gap-2">
              <Input
                placeholder="Бассейн, парная, веники..."
                value={newAmenity}
                onChange={(e) => setNewAmenity(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    addAmenity();
                  }
                }}
              />
              <Button type="button" variant="outline" onClick={addAmenity}>
                <Plus className="h-4 w-4" />
              </Button>
            </div>
          </CardContent>
        </Card>

        <VenueServicesFields
          value={form.services}
          onChange={(next) => updateField("services", next)}
        />

        {error && (
          <p className="text-sm text-destructive">{error}</p>
        )}

        <Button type="submit" className="w-full" size="lg" disabled={loading}>
          {loading ? "Создание..." : "Создать заведение"}
        </Button>
      </form>
    </div>
  );
}
