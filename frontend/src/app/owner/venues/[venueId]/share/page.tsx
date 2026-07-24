"use client";

import { useEffect, useMemo } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import {
  CalendarCheck,
  Copy,
  Info,
  Link2,
  MessageCircle,
  Send,
} from "lucide-react";
import { redirectToLogin } from "@/lib/auth-redirect";
import { getOwnerVenues, venueMediaUrl } from "@/lib/api";
import { widgetSeenKey } from "@/components/banya/onboarding-checklist";
import { siteUrl } from "@/lib/seo-site";
import { useAuthStore } from "@/store/auth";
import { VENUE_TYPE_LABELS } from "@/lib/types";

const uuidRe =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function formatMoney(v: number): string {
  return `${new Intl.NumberFormat("ru-RU").format(Math.round(v))} ₽`;
}

export default function OwnerVenueSharePage() {
  const params = useParams<{ venueId: string }>();
  const venueId = params.venueId;
  const { token, user, hydrated } = useAuthStore();

  const validId = typeof venueId === "string" && uuidRe.test(venueId);
  const canOwnerCabinet =
    user?.role === "venue_owner" ||
    user?.role === "master" ||
    user?.role === "user";

  useEffect(() => {
    if (hydrated && (!token || !canOwnerCabinet)) redirectToLogin();
  }, [hydrated, token, canOwnerCabinet]);

  // Отмечаем шаг онбординга «виджет брони» выполненным по факту открытия раздела.
  useEffect(() => {
    if (validId) localStorage.setItem(widgetSeenKey(venueId), "1");
  }, [validId, venueId]);

  const { data: venues, isLoading } = useQuery({
    queryKey: ["owner-venues"],
    queryFn: getOwnerVenues,
    enabled: !!token && canOwnerCabinet && validId,
  });
  const venue = useMemo(
    () => venues?.find((v) => v.id === venueId) ?? null,
    [venues, venueId],
  );

  // Публичная ссылка на карточку бани с формой брони.
  const bookingUrl = venue ? `${siteUrl()}/venues/${venue.slug}` : "";
  const shareText = venue
    ? `Забронировать баню «${venue.name}» онлайн`
    : "";

  const cover = useMemo(() => {
    const photos = venue?.photos ?? [];
    const raw = (photos.find((p) => p.is_cover) ?? photos[0])?.url;
    return raw ? venueMediaUrl(raw) : null;
  }, [venue]);

  const embedCode = venue
    ? `<iframe src="${siteUrl()}/embed/${venue.slug}" width="100%" height="360" style="border:0;max-width:440px" title="Забронировать — ${venue.name}" loading="lazy"></iframe>`
    : "";

  const copyText = async (text: string, ok: string) => {
    try {
      await navigator.clipboard.writeText(text);
      toast.success(ok);
    } catch {
      toast.error("Не удалось скопировать. Скопируйте вручную.");
    }
  };

  const shareLinks = useMemo(() => {
    const u = encodeURIComponent(bookingUrl);
    const t = encodeURIComponent(shareText);
    return {
      telegram: `https://t.me/share/url?url=${u}&text=${t}`,
      vk: `https://vk.com/share.php?url=${u}`,
      whatsapp: `https://wa.me/?text=${encodeURIComponent(`${shareText} ${bookingUrl}`)}`,
    };
  }, [bookingUrl, shareText]);

  if (!hydrated || !token) return null;

  if (!validId) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-10">
        <p className="text-sm text-muted-foreground">Некорректная ссылка.</p>
        <Button asChild variant="link" className="mt-2 h-auto px-0 text-sm">
          <Link href="/owner/venues">← Панель</Link>
        </Button>
      </div>
    );
  }

  if (isLoading || !venues) {
    return (
      <div className="p-4 md:p-6">
        <div className="h-40 animate-pulse rounded-lg border border-border bg-muted" />
      </div>
    );
  }

  if (!venue) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-10">
        <p className="text-sm text-destructive">
          Заведение не найдено в вашем доступе.
        </p>
        <Button asChild variant="link" className="mt-2 h-auto px-0 text-sm">
          <Link href="/owner/venues">← Панель</Link>
        </Button>
      </div>
    );
  }

  return (
    <div className="p-4 md:p-6">
      <div className="mb-5">
        <h1 className="text-2xl font-bold text-foreground">Виджет брони</h1>
        <p className="text-sm text-muted-foreground">
          {venue.name} · дайте гостям ссылку — форма брони с предоплатой уже готова
        </p>
      </div>

      <div className="grid gap-4 lg:grid-cols-[1.4fr_1fr]">
        <div className="space-y-4">
          <Card className="border-border">
            <CardHeader>
              <CardTitle>Ссылка на бронь</CardTitle>
              <CardDescription>
                Вставьте её в группу VK, канал Telegram, на сайт или в описание —
                настраивать ничего не нужно.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex flex-wrap items-center gap-2">
                <div className="flex min-w-[220px] flex-1 items-center gap-2 rounded-lg border border-border bg-muted/40 px-3 py-2.5 text-sm">
                  <Link2 className="h-4 w-4 shrink-0 text-muted-foreground" />
                  <span className="truncate">{bookingUrl}</span>
                </div>
                <Button
                  type="button"
                  onClick={() => copyText(bookingUrl, "Ссылка скопирована")}
                  className="gap-1.5"
                >
                  <Copy className="h-4 w-4" />
                  Скопировать
                </Button>
              </div>

              <div>
                <p className="mb-2 text-sm text-muted-foreground">
                  Поделиться в один клик
                </p>
                <div className="flex flex-wrap gap-2">
                  <Button asChild variant="outline" className="gap-1.5">
                    <a
                      href={shareLinks.telegram}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      <Send className="h-4 w-4" />
                      Telegram
                    </a>
                  </Button>
                  <Button asChild variant="outline" className="gap-1.5">
                    <a
                      href={shareLinks.vk}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      <Link2 className="h-4 w-4" />
                      ВКонтакте
                    </a>
                  </Button>
                  <Button asChild variant="outline" className="gap-1.5">
                    <a
                      href={shareLinks.whatsapp}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      <MessageCircle className="h-4 w-4" />
                      WhatsApp
                    </a>
                  </Button>
                </div>
              </div>

              <p className="flex items-start gap-1.5 text-xs text-muted-foreground">
                <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                Гость видит свободное время сразу; предоплата защищает бронь от
                неявки.
              </p>
            </CardContent>
          </Card>

          <Card className="border-border">
            <CardHeader>
              <CardTitle className="text-base">Виджет для своего сайта</CardTitle>
              <CardDescription>
                Вставьте этот код на страницу — карточка с кнопкой брони появится
                прямо на вашем сайте.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <pre className="max-w-md overflow-x-auto rounded-lg border border-border bg-muted/40 px-3 py-2.5 text-xs text-muted-foreground">
                {embedCode}
              </pre>
              <Button
                type="button"
                variant="outline"
                onClick={() => copyText(embedCode, "Код скопирован")}
                className="gap-1.5"
              >
                <Copy className="h-4 w-4" />
                Скопировать код
              </Button>
            </CardContent>
          </Card>
        </div>

        <Card className="border-border bg-muted/30">
          <CardHeader>
            <CardTitle className="text-base">Так это увидят гости</CardTitle>
            <CardDescription>Превью в вашей группе или канале</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="overflow-hidden rounded-xl border border-border bg-card">
              <div className="flex h-32 items-center justify-center bg-secondary text-xs text-secondary-foreground">
                {cover ? (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img
                    src={cover}
                    alt=""
                    className="h-full w-full object-cover"
                  />
                ) : (
                  <span>Добавьте фото на странице «Заведение»</span>
                )}
              </div>
              <div className="p-4">
                <div className="font-medium text-foreground">{venue.name}</div>
                <div className="mt-0.5 text-sm text-muted-foreground">
                  {venue.city} · {VENUE_TYPE_LABELS[venue.type] ?? venue.type} · от{" "}
                  {formatMoney(venue.price_from)}
                </div>
                <Button className="mt-3 w-full gap-1.5" disabled>
                  <CalendarCheck className="h-4 w-4" />
                  Забронировать онлайн
                </Button>
                <p className="mt-2 text-center text-xs text-muted-foreground">
                  Свободное время видно сразу
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
