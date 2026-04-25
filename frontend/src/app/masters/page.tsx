"use client";

import Image from "next/image";
import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { listPublicMasters, masterCardImageSrc, masterCardPriceLabel } from "@/lib/api";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Users } from "lucide-react";

export default function MastersCatalogPage() {
  const { data, isLoading } = useQuery({
    queryKey: ["public-masters"],
    queryFn: () => listPublicMasters({ limit: 100 }),
  });

  const masters = data?.masters ?? [];

  return (
    <div className="container mx-auto px-4 py-10">
      <div className="mb-8 flex items-center gap-3">
        <div className="flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
          <Users className="h-6 w-6 text-primary" />
        </div>
        <div>
          <h1 className="text-2xl font-bold">Пар-мастера</h1>
          <p className="text-muted-foreground">Проверенные специалисты на платформе БаняГид</p>
        </div>
      </div>

      {isLoading && <p className="text-muted-foreground">Загрузка...</p>}

      {!isLoading && masters.length === 0 && (
        <p className="text-muted-foreground">В каталоге пока нет опубликованных мастеров.</p>
      )}

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {masters.map((m) => {
          const priceLine = masterCardPriceLabel(m);
          const img = masterCardImageSrc(m);
          return (
            <Link key={m.id} href={`/masters/${m.slug}`}>
              <Card className="h-full overflow-hidden transition-colors hover:border-primary/40">
                {img ? (
                  <div className="relative aspect-[5/3] w-full bg-muted">
                    <Image
                      src={img}
                      alt=""
                      fill
                      className="object-cover"
                      sizes="(max-width: 768px) 100vw, 33vw"
                      unoptimized
                    />
                  </div>
                ) : null}
                <CardContent className="p-5">
                  <h2 className="font-semibold text-lg mb-1">{m.display_name}</h2>
                  <p className="text-sm text-muted-foreground mb-2">{m.city}</p>
                  {m.experience_years > 0 && (
                    <Badge variant="outline" className="text-xs">
                      Опыт {m.experience_years} лет
                    </Badge>
                  )}
                  {priceLine ? (
                    <p className="mt-2 text-sm font-medium text-foreground">{priceLine}</p>
                  ) : null}
                  <p className="mt-3 line-clamp-2 text-sm text-muted-foreground">{m.bio}</p>
                </CardContent>
              </Card>
            </Link>
          );
        })}
      </div>
    </div>
  );
}
