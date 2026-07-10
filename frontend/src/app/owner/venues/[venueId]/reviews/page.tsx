"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import {
  CornerDownRight,
  Loader2,
  MessageSquareOff,
  Pencil,
  Send,
  Star,
  Trash2,
  User as UserIcon,
} from "lucide-react";
import { redirectToLogin } from "@/lib/auth-redirect";
import {
  deleteVenueReviewReply,
  getOwnerVenueReviews,
  getOwnerVenueReviewSummary,
  getOwnerVenues,
  replyToVenueReview,
} from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import type { Review } from "@/lib/types";

const uuidRe =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

const PAGE_SIZE = 20;

function formatDate(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("ru-RU", {
    day: "2-digit",
    month: "short",
    year: "numeric",
  });
}

function Stars({ rating }: { rating: number }) {
  return (
    <span className="inline-flex" aria-label={`Оценка ${rating} из 5`}>
      {[1, 2, 3, 4, 5].map((n) => (
        <Star
          key={n}
          className={
            n <= rating
              ? "h-4 w-4 fill-amber-500 text-amber-500"
              : "h-4 w-4 text-muted-foreground/40"
          }
        />
      ))}
    </span>
  );
}

export default function OwnerVenueReviewsPage() {
  const params = useParams<{ venueId: string }>();
  const venueId = params.venueId;
  const { token, user, hydrated } = useAuthStore();

  const [onlyUnanswered, setOnlyUnanswered] = useState(false);

  const validId = typeof venueId === "string" && uuidRe.test(venueId);
  const canOwnerCabinet =
    user?.role === "venue_owner" ||
    user?.role === "master" ||
    user?.role === "user";

  useEffect(() => {
    if (hydrated && (!token || !canOwnerCabinet)) {
      redirectToLogin();
    }
  }, [hydrated, token, canOwnerCabinet]);

  const { data: venues, isLoading: venuesLoading } = useQuery({
    queryKey: ["owner-venues"],
    queryFn: getOwnerVenues,
    enabled: !!token && canOwnerCabinet && validId,
  });

  const venue = useMemo(
    () => venues?.find((v) => v.id === venueId) ?? null,
    [venues, venueId],
  );

  const enabled = !!token && validId && !!venue;

  const { data: summary } = useQuery({
    queryKey: ["owner-review-summary", venueId],
    queryFn: () => getOwnerVenueReviewSummary(venueId as string),
    enabled,
  });

  const {
    data,
    fetchNextPage,
    hasNextPage,
    isFetching,
    isFetchingNextPage,
    isLoading,
  } = useInfiniteQuery({
    queryKey: ["owner-reviews", venueId, onlyUnanswered],
    queryFn: ({ pageParam }) =>
      getOwnerVenueReviews(venueId as string, {
        only_unanswered: onlyUnanswered,
        page: pageParam,
        page_size: PAGE_SIZE,
      }),
    initialPageParam: 1,
    getNextPageParam: (lastPage, allPages) => {
      const loaded = allPages.reduce((n, p) => n + p.reviews.length, 0);
      return loaded < lastPage.total ? allPages.length + 1 : undefined;
    },
    enabled,
  });

  const items = useMemo(
    () => data?.pages.flatMap((p) => p.reviews) ?? [],
    [data],
  );

  // Reply/delete invalidate the feed (so the only_unanswered view drops answered
  // reviews) and the summary counters. Refetching the loaded pages is cheap here.
  const queryClient = useQueryClient();
  const onChanged = () => {
    queryClient.invalidateQueries({ queryKey: ["owner-reviews", venueId] });
    queryClient.invalidateQueries({
      queryKey: ["owner-review-summary", venueId],
    });
  };

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

  if (venuesLoading || !venues) {
    return (
      <section className="py-4 md:py-6">
        <div className="container mx-auto max-w-3xl px-4">
          <div className="h-40 animate-pulse rounded-lg border border-border bg-muted" />
        </div>
      </section>
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
    <section className="py-4 md:py-6">
      <div className="container mx-auto max-w-3xl px-4">
        <div className="mb-4">
          <h1 className="text-2xl font-bold text-foreground md:text-3xl">
            Отзывы: {venue.name}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Читайте отзывы гостей и отвечайте на них — ответ увидят будущие
            клиенты.
          </p>
        </div>

        {summary && summary.review_count > 0 ? (
          <div className="mb-4 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-muted-foreground">
            <span className="inline-flex items-center gap-1.5">
              <span className="text-lg font-semibold text-foreground">
                {summary.avg_rating.toFixed(1)}
              </span>
              <Stars rating={Math.round(summary.avg_rating)} />
            </span>
            <span className="text-border">·</span>
            <span>{summary.review_count} отзывов</span>
            {summary.unanswered_count > 0 ? (
              <>
                <span className="text-border">·</span>
                <span className="text-destructive">
                  {summary.unanswered_count} без ответа
                </span>
              </>
            ) : null}
          </div>
        ) : null}

        <div className="mb-4 flex items-center gap-2">
          <Button
            type="button"
            size="sm"
            variant={onlyUnanswered ? "outline" : "default"}
            onClick={() => setOnlyUnanswered(false)}
          >
            Все отзывы
          </Button>
          <Button
            type="button"
            size="sm"
            variant={onlyUnanswered ? "default" : "outline"}
            onClick={() => setOnlyUnanswered(true)}
          >
            <MessageSquareOff className="mr-1.5 h-4 w-4" />
            Только без ответа
          </Button>
        </div>

        {isLoading ? (
          <div className="flex items-center justify-center gap-2 rounded-lg border border-border p-10 text-muted-foreground">
            <Loader2 className="h-5 w-5 animate-spin" />
            Загрузка…
          </div>
        ) : items.length === 0 ? (
          <p className="rounded-lg border border-border p-8 text-sm text-muted-foreground">
            {onlyUnanswered
              ? "Все отзывы получили ответ. Отличная работа!"
              : "Отзывов пока нет. Они появятся здесь после первых визитов гостей."}
          </p>
        ) : (
          <div className="space-y-3">
            {items.map((review) => (
              <ReviewCard
                key={review.id}
                venueId={venueId as string}
                review={review}
                onChanged={onChanged}
              />
            ))}
          </div>
        )}

        {hasNextPage ? (
          <div className="mt-4 text-center">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={isFetching}
              onClick={() => fetchNextPage()}
            >
              {isFetchingNextPage ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                "Показать ещё"
              )}
            </Button>
          </div>
        ) : null}
      </div>
    </section>
  );
}

function ReviewCard({
  venueId,
  review,
  onChanged,
}: {
  venueId: string;
  review: Review;
  onChanged: () => void;
}) {
  const reply = review.owner_reply;
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(reply?.body ?? "");

  const replyMutation = useMutation({
    mutationFn: () => replyToVenueReview(venueId, review.id, draft.trim()),
    onSuccess: () => {
      setEditing(false);
      onChanged();
    },
  });

  const deleteMutation = useMutation({
    mutationFn: () => deleteVenueReviewReply(venueId, review.id),
    onSuccess: () => {
      setDraft("");
      onChanged();
    },
  });

  const anonymous = review.is_anonymous || !review.user_name;
  const initials = anonymous
    ? ""
    : review.user_name
        .split(/\s+/)
        .slice(0, 2)
        .map((w) => w[0]?.toUpperCase() ?? "")
        .join("");

  const showComposer = editing || !reply;

  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="flex items-center gap-3">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary/10 text-sm font-medium text-primary">
          {anonymous ? (
            <UserIcon className="h-4 w-4 text-muted-foreground" />
          ) : (
            initials
          )}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium text-foreground">
              {anonymous ? "Аноним" : review.user_name}
            </span>
            {review.verified ? (
              <Badge
                variant="secondary"
                className="border-transparent bg-emerald-500/10 font-normal text-emerald-700 dark:text-emerald-400"
              >
                Проверен
              </Badge>
            ) : null}
          </div>
          <div className="text-xs text-muted-foreground">
            {formatDate(review.created_at)}
          </div>
        </div>
        <Stars rating={review.rating} />
      </div>

      {review.text ? (
        <p className="mt-2.5 whitespace-pre-wrap text-sm leading-relaxed text-foreground">
          {review.text}
        </p>
      ) : null}

      {reply && !editing ? (
        <div className="mt-3 rounded-r-lg border-l-2 border-primary bg-muted/40 px-3.5 py-2.5">
          <div className="flex items-center gap-1.5 text-xs font-medium text-primary">
            <CornerDownRight className="h-3.5 w-3.5" />
            Ответ заведения
          </div>
          <p className="mt-1 whitespace-pre-wrap text-sm leading-relaxed text-muted-foreground">
            {reply.body}
          </p>
          <div className="mt-1.5 flex items-center gap-4">
            <button
              type="button"
              className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
              onClick={() => {
                setDraft(reply.body);
                setEditing(true);
              }}
            >
              <Pencil className="h-3 w-3" /> Изменить
            </button>
            <button
              type="button"
              className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-destructive disabled:opacity-50"
              disabled={deleteMutation.isPending}
              onClick={() => deleteMutation.mutate()}
            >
              {deleteMutation.isPending ? (
                <Loader2 className="h-3 w-3 animate-spin" />
              ) : (
                <Trash2 className="h-3 w-3" />
              )}{" "}
              Удалить
            </button>
          </div>
        </div>
      ) : null}

      {showComposer ? (
        <div className="mt-3">
          <Textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder="Ответьте гостю от лица заведения — вежливо и по делу"
            rows={2}
          />
          <div className="mt-2 flex items-center justify-end gap-2">
            {replyMutation.isError ? (
              <span className="mr-auto text-xs text-destructive">
                Не удалось сохранить ответ.
              </span>
            ) : null}
            {editing ? (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => {
                  setEditing(false);
                  setDraft(reply?.body ?? "");
                }}
              >
                Отмена
              </Button>
            ) : null}
            <Button
              type="button"
              size="sm"
              disabled={!draft.trim() || replyMutation.isPending}
              onClick={() => replyMutation.mutate()}
            >
              {replyMutation.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <>
                  <Send className="mr-1.5 h-4 w-4" />
                  {editing ? "Сохранить" : "Опубликовать ответ"}
                </>
              )}
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  );
}
