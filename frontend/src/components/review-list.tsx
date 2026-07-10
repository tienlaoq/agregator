"use client";

import { useState } from "react";
import { StarRating } from "@/components/star-rating";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ApiError, createMasterReview, createReview, formatApiErrorMessage } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import type { Review } from "@/lib/types";
import { CheckCircle, CornerDownRight, MessageSquare } from "lucide-react";

interface ReviewListProps {
  targetId: string;
  targetType?: "venue" | "master";
  reviews: Review[];
  onReviewAdded?: () => void;
}

function RatingDistribution({ reviews }: { reviews: Review[] }) {
  const total = reviews.length;
  const counts = [5, 4, 3, 2, 1].map((star) => ({
    star,
    count: reviews.filter((r) => Math.round(r.rating) === star).length,
  }));

  if (total === 0) return null;

  return (
    <div className="space-y-1.5">
      {counts.map(({ star, count }) => (
        <div key={star} className="flex items-center gap-2 text-sm">
          <span className="w-3 text-right text-muted-foreground">{star}</span>
          <div className="h-2 flex-1 rounded-full bg-muted">
            <div
              className="h-2 rounded-full bg-amber-500 transition-all"
              style={{ width: `${total ? (count / total) * 100 : 0}%` }}
            />
          </div>
          <span className="w-8 text-right text-xs text-muted-foreground">
            {count}
          </span>
        </div>
      ))}
    </div>
  );
}

export function ReviewList({ targetId, targetType = "venue", reviews, onReviewAdded }: ReviewListProps) {
  const { token } = useAuthStore();
  const [showForm, setShowForm] = useState(false);
  const [rating, setRating] = useState(5);
  const [text, setText] = useState("");
  const [loading, setLoading] = useState(false);
  const [submitError, setSubmitError] = useState("");
  const [isAnonymous, setIsAnonymous] = useState(false);

  const avgRating =
    reviews.length > 0
      ? reviews.reduce((sum, r) => sum + r.rating, 0) / reviews.length
      : 0;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setSubmitError("");
    try {
      if (targetType === "master") {
        await createMasterReview(targetId, { rating, text, is_anonymous: isAnonymous });
      } else {
        await createReview(targetId, { rating, text, is_anonymous: isAnonymous });
      }
      setText("");
      setRating(5);
      setIsAnonymous(false);
      setShowForm(false);
      onReviewAdded?.();
    } catch (e) {
      if (e instanceof ApiError && e.code === "GATEWAY.UPSTREAM.ALREADY_EXISTS") {
        setSubmitError(
          targetType === "master"
            ? "Вы уже оставляли отзыв этому мастеру."
            : "Вы уже оставляли отзыв этому заведению.",
        );
        return;
      }
      setSubmitError(
        formatApiErrorMessage(
          e,
          "Не удалось отправить отзыв. Попробуйте позже.",
        ),
      );
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-2">
          <div className="flex items-center gap-3">
            <span className="text-3xl font-bold">{avgRating.toFixed(1)}</span>
            <div>
              <StarRating rating={avgRating} size="md" />
              <p className="mt-0.5 text-xs text-muted-foreground">
                {reviews.length} отзыв(ов)
              </p>
            </div>
          </div>
          <RatingDistribution reviews={reviews} />
        </div>
        {token && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setShowForm(!showForm);
              setSubmitError("");
              if (showForm) setIsAnonymous(false);
            }}
          >
            <MessageSquare className="mr-1 h-4 w-4" />
            Написать отзыв
          </Button>
        )}
      </div>

      {showForm && (
        <form
          onSubmit={handleSubmit}
          className="space-y-3 rounded-lg border bg-muted/30 p-4"
        >
          <div className="space-y-2">
            <Label>Ваша оценка</Label>
            <StarRating
              rating={rating}
              size="lg"
              interactive
              onChange={setRating}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="review-text">Комментарий</Label>
            <Textarea
              id="review-text"
              placeholder="Расскажите о вашем опыте..."
              value={text}
              onChange={(e) => setText(e.target.value)}
              required
            />
          </div>
          <div className="flex items-center gap-2">
            <Checkbox
              id="review-anonymous"
              checked={isAnonymous}
              onCheckedChange={(v) => setIsAnonymous(v === true)}
            />
            <Label htmlFor="review-anonymous" className="cursor-pointer text-sm font-normal">
              Опубликовать отзыв анонимно
            </Label>
          </div>
          <div className="flex gap-2">
            <Button type="submit" size="sm" disabled={loading}>
              {loading ? "Отправка..." : "Отправить"}
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => {
                setShowForm(false);
                setIsAnonymous(false);
              }}
            >
              Отмена
            </Button>
          </div>
          {submitError ? (
            <p className="text-sm text-destructive">{submitError}</p>
          ) : null}
        </form>
      )}

      <Separator />

      {reviews.length === 0 ? (
        <p className="py-8 text-center text-muted-foreground">
          Пока нет отзывов. Будьте первым!
        </p>
      ) : (
        <div className="space-y-4">
          {reviews.map((review) => {
            const name = review.is_anonymous
              ? "Аноним"
              : (review.user_name || "").trim() || "Пользователь";
            return (
              <div key={review.id} className="space-y-2">
              <div className="flex items-center gap-2">
                <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary/10 text-sm font-medium text-primary">
                  {name.charAt(0).toUpperCase()}
                </div>
                <div className="flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">{name}</span>
                    {review.verified && (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Badge variant="secondary" className="gap-1 text-xs">
                            <CheckCircle className="h-3 w-3" />
                            Проверено
                          </Badge>
                        </TooltipTrigger>
                        <TooltipContent side="top" sideOffset={6} className="max-w-xs text-[11px] leading-relaxed">
                          Метка ставится автоматически: у автора есть подтверждённый завершённый визит в это
                          заведение, поэтому отзыв считается подтверждённым.
                        </TooltipContent>
                      </Tooltip>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    <StarRating rating={review.rating} size="sm" />
                    <span className="text-xs text-muted-foreground">
                      {new Date(review.created_at).toLocaleDateString("ru-RU")}
                    </span>
                  </div>
                </div>
              </div>
              <p className="text-sm text-foreground/80">{review.text}</p>
              {review.owner_reply ? (
                <div className="ml-10 rounded-r-lg border-l-2 border-primary bg-muted/40 px-3.5 py-2.5">
                  <div className="flex items-center gap-1.5 text-xs font-medium text-primary">
                    <CornerDownRight className="h-3.5 w-3.5" />
                    Ответ заведения
                  </div>
                  <p className="mt-1 whitespace-pre-wrap text-sm leading-relaxed text-muted-foreground">
                    {review.owner_reply.body}
                  </p>
                </div>
              ) : null}
              <Separator />
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
