"use client";

import { useState } from "react";
import { StarRating } from "@/components/star-rating";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Badge } from "@/components/ui/badge";
import { createReview } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import type { Review } from "@/lib/types";
import { CheckCircle, MessageSquare } from "lucide-react";

interface ReviewListProps {
  venueId: string;
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

export function ReviewList({ venueId, reviews, onReviewAdded }: ReviewListProps) {
  const { token } = useAuthStore();
  const [showForm, setShowForm] = useState(false);
  const [rating, setRating] = useState(5);
  const [text, setText] = useState("");
  const [loading, setLoading] = useState(false);

  const avgRating =
    reviews.length > 0
      ? reviews.reduce((sum, r) => sum + r.rating, 0) / reviews.length
      : 0;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      await createReview(venueId, { rating, text });
      setText("");
      setRating(5);
      setShowForm(false);
      onReviewAdded?.();
    } catch {
      // silently fail for MVP
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
            onClick={() => setShowForm(!showForm)}
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
          <div className="flex gap-2">
            <Button type="submit" size="sm" disabled={loading}>
              {loading ? "Отправка..." : "Отправить"}
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => setShowForm(false)}
            >
              Отмена
            </Button>
          </div>
        </form>
      )}

      <Separator />

      {reviews.length === 0 ? (
        <p className="py-8 text-center text-muted-foreground">
          Пока нет отзывов. Будьте первым!
        </p>
      ) : (
        <div className="space-y-4">
          {reviews.map((review) => (
            <div key={review.id} className="space-y-2">
              <div className="flex items-center gap-2">
                <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary/10 text-sm font-medium text-primary">
                  {review.user_name.charAt(0).toUpperCase()}
                </div>
                <div className="flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">{review.user_name}</span>
                    {review.verified && (
                      <Badge variant="secondary" className="gap-1 text-xs">
                        <CheckCircle className="h-3 w-3" />
                        Проверено
                      </Badge>
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
              <Separator />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
