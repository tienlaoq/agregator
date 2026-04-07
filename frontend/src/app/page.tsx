"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { VenueCard } from "@/components/venue-card";
import { getVenues } from "@/lib/api";
import { Search, Flame, MapPin, Shield } from "lucide-react";

export default function HomePage() {
  const [search, setSearch] = useState("");
  const router = useRouter();

  const { data: venues } = useQuery({
    queryKey: ["venues", "popular"],
    queryFn: () => getVenues(),
  });

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    router.push(`/venues?q=${encodeURIComponent(search)}`);
  };

  const popularVenues = venues?.slice(0, 6) ?? [];

  return (
    <div className="flex flex-col">
      {/* Hero */}
      <section className="relative overflow-hidden bg-gradient-to-br from-amber-600 via-orange-500 to-amber-700 px-4 py-20 text-white md:py-32">
        <div className="absolute inset-0 bg-[url('/placeholder.svg')] opacity-5" />
        <div className="relative mx-auto max-w-3xl text-center">
          <h1 className="mb-4 text-4xl font-bold tracking-tight md:text-6xl">
            Найдите идеальную баню
          </h1>
          <p className="mb-8 text-lg text-amber-100 md:text-xl">
            Лучшие бани, сауны и хаммамы вашего города в одном месте
          </p>
          <form
            onSubmit={handleSearch}
            className="mx-auto flex max-w-xl gap-2"
          >
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Город или название..."
                className="h-11 bg-white pl-10 text-foreground"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
            </div>
            <Button
              type="submit"
              size="lg"
              className="h-11 bg-white text-amber-700 hover:bg-amber-50"
            >
              Найти
            </Button>
          </form>
        </div>
      </section>

      {/* Features */}
      <section className="border-b bg-muted/30 px-4 py-12">
        <div className="mx-auto grid max-w-5xl gap-8 sm:grid-cols-3">
          {[
            {
              icon: Flame,
              title: "500+ заведений",
              desc: "Бани, сауны и хаммамы по всей России",
            },
            {
              icon: MapPin,
              title: "Удобный поиск",
              desc: "Фильтры по типу, цене и рейтингу",
            },
            {
              icon: Shield,
              title: "Проверенные отзывы",
              desc: "Реальные оценки от посетителей",
            },
          ].map((f) => (
            <div key={f.title} className="flex flex-col items-center text-center">
              <div className="mb-3 flex h-12 w-12 items-center justify-center rounded-xl bg-primary/10">
                <f.icon className="h-6 w-6 text-primary" />
              </div>
              <h3 className="mb-1 font-semibold">{f.title}</h3>
              <p className="text-sm text-muted-foreground">{f.desc}</p>
            </div>
          ))}
        </div>
      </section>

      {/* Popular venues */}
      <section className="px-4 py-12">
        <div className="mx-auto max-w-7xl">
          <div className="mb-8 flex items-center justify-between">
            <h2 className="text-2xl font-bold">Популярные бани</h2>
            <Button
              variant="ghost"
              onClick={() => router.push("/venues")}
            >
              Смотреть все →
            </Button>
          </div>
          {popularVenues.length > 0 ? (
            <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
              {popularVenues.map((venue) => (
                <VenueCard key={venue.id} venue={venue} />
              ))}
            </div>
          ) : (
            <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
              {MOCK_VENUES.map((venue) => (
                <VenueCard key={venue.id} venue={venue} />
              ))}
            </div>
          )}
        </div>
      </section>

      {/* CTA */}
      <section className="bg-gradient-to-r from-amber-50 to-orange-50 px-4 py-12">
        <div className="mx-auto max-w-2xl text-center">
          <h2 className="mb-3 text-2xl font-bold">Вы владелец бани?</h2>
          <p className="mb-6 text-muted-foreground">
            Добавьте своё заведение на БаняГид и привлекайте новых клиентов
          </p>
          <Button size="lg" onClick={() => router.push("/auth/register")}>
            Зарегистрироваться как владелец
          </Button>
        </div>
      </section>
    </div>
  );
}

const MOCK_VENUES = [
  {
    id: "1",
    slug: "russkaya-banya-na-presne",
    name: "Русская баня на Пресне",
    type: "banya" as const,
    description: "Традиционная русская баня",
    address: "ул. Пресненский Вал, 15, Москва",
    city: "Москва",
    phone: "+7 495 123-45-67",
    price_from: 1500,
    rating: 4.7,
    review_count: 124,
    amenities: [],
    services: [],
    owner_id: "1",
  },
  {
    id: "2",
    slug: "sauna-relax",
    name: "Сауна Релакс",
    type: "sauna" as const,
    description: "Финская сауна",
    address: "пр. Невский, 88, Санкт-Петербург",
    city: "Санкт-Петербург",
    phone: "+7 812 765-43-21",
    price_from: 2000,
    rating: 4.5,
    review_count: 89,
    amenities: [],
    services: [],
    owner_id: "2",
  },
  {
    id: "3",
    slug: "hammam-sultan",
    name: "Хаммам Султан",
    type: "hammam" as const,
    description: "Настоящий турецкий хаммам",
    address: "ул. Тверская, 22, Москва",
    city: "Москва",
    phone: "+7 495 987-65-43",
    price_from: 3000,
    rating: 4.9,
    review_count: 56,
    amenities: [],
    services: [],
    owner_id: "3",
  },
];
