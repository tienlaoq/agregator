"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { createVenue } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import { VENUE_TYPE_LABELS } from "@/lib/types";
import type { CreateVenueRequest } from "@/lib/types";
import { Plus, X } from "lucide-react";

export default function CreateVenuePage() {
  const router = useRouter();
  const { token, user, hydrated } = useAuthStore();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const [form, setForm] = useState<CreateVenueRequest>({
    name: "",
    type: "banya",
    address: "",
    city: "",
    description: "",
    phone: "",
    price_from: 0,
    amenities: [],
    services: [],
  });

  const [newAmenity, setNewAmenity] = useState("");
  const [newService, setNewService] = useState({
    name: "",
    description: "",
    price: 0,
    duration_minutes: 60,
  });

  useEffect(() => {
    if (hydrated && (!token || user?.role !== "owner")) {
      router.push("/auth/login");
    }
  }, [hydrated, token, user, router]);

  const updateField = <K extends keyof CreateVenueRequest>(
    key: K,
    value: CreateVenueRequest[K],
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

  const addService = () => {
    if (newService.name.trim()) {
      updateField("services", [...form.services, { ...newService }]);
      setNewService({ name: "", description: "", price: 0, duration_minutes: 60 });
    }
  };

  const removeService = (index: number) => {
    updateField(
      "services",
      form.services.filter((_, i) => i !== index),
    );
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      await createVenue(form);
      router.push("/owner/venues");
    } catch {
      setError("Не удалось создать заведение. Попробуйте позже.");
    } finally {
      setLoading(false);
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
                      updateField("type", value as CreateVenueRequest["type"])
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
                <Label htmlFor="city">Город</Label>
                <Input
                  id="city"
                  placeholder="Москва"
                  value={form.city}
                  onChange={(e) => updateField("city", e.target.value)}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="phone">Телефон</Label>
                <Input
                  id="phone"
                  type="tel"
                  placeholder="+7 495 123-45-67"
                  value={form.phone}
                  onChange={(e) => updateField("phone", e.target.value)}
                  required
                />
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="address">Адрес</Label>
              <Input
                id="address"
                placeholder="ул. Пример, д. 1"
                value={form.address}
                onChange={(e) => updateField("address", e.target.value)}
                required
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

            <div className="space-y-2">
              <Label htmlFor="price">Цена от (₽/час)</Label>
              <Input
                id="price"
                type="number"
                min={0}
                placeholder="1500"
                value={form.price_from || ""}
                onChange={(e) => updateField("price_from", Number(e.target.value))}
                required
              />
            </div>
          </CardContent>
        </Card>

        {/* Amenities */}
        <Card>
          <CardHeader>
            <CardTitle>Удобства</CardTitle>
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

        {/* Services */}
        <Card>
          <CardHeader>
            <CardTitle>Услуги</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {form.services.map((s, i) => (
              <div
                key={i}
                className="flex items-center justify-between rounded-lg border p-3"
              >
                <div>
                  <p className="font-medium">{s.name}</p>
                  <p className="text-xs text-muted-foreground">
                    {s.duration_minutes} мин &middot;{" "}
                    {s.price.toLocaleString("ru-RU")} ₽
                  </p>
                </div>
                <button
                  type="button"
                  onClick={() => removeService(i)}
                  className="text-muted-foreground hover:text-destructive"
                >
                  <X className="h-4 w-4" />
                </button>
              </div>
            ))}

            <Separator />

            <div className="space-y-3 rounded-lg bg-muted/30 p-3">
              <p className="text-sm font-medium">Добавить услугу</p>
              <div className="grid gap-3 sm:grid-cols-2">
                <Input
                  placeholder="Название услуги"
                  value={newService.name}
                  onChange={(e) =>
                    setNewService({ ...newService, name: e.target.value })
                  }
                />
                <Input
                  placeholder="Описание"
                  value={newService.description}
                  onChange={(e) =>
                    setNewService({ ...newService, description: e.target.value })
                  }
                />
                <Input
                  type="number"
                  placeholder="Цена (₽)"
                  value={newService.price || ""}
                  onChange={(e) =>
                    setNewService({
                      ...newService,
                      price: Number(e.target.value),
                    })
                  }
                />
                <Input
                  type="number"
                  placeholder="Длительность (мин)"
                  value={newService.duration_minutes || ""}
                  onChange={(e) =>
                    setNewService({
                      ...newService,
                      duration_minutes: Number(e.target.value),
                    })
                  }
                />
              </div>
              <Button type="button" variant="outline" size="sm" onClick={addService}>
                <Plus className="mr-1 h-4 w-4" />
                Добавить
              </Button>
            </div>
          </CardContent>
        </Card>

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
