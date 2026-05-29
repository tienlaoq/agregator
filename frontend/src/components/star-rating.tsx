import { Star } from "lucide-react";
import { cn } from "@/lib/utils";

interface StarRatingProps {
  rating: number;
  max?: number;
  size?: "sm" | "md" | "lg";
  showValue?: boolean;
  interactive?: boolean;
  onChange?: (rating: number) => void;
}

const sizeMap = {
  sm: "h-3.5 w-3.5",
  md: "h-4 w-4",
  lg: "h-5 w-5",
};

export function StarRating({
  rating,
  max = 5,
  size = "md",
  showValue = false,
  interactive = false,
  onChange,
}: StarRatingProps) {
  const rounded = Math.round(rating);

  if (interactive) {
    // Интерактивный режим: radiogroup, стрелки ← → меняют оценку.
    return (
      <div className="flex items-center gap-1">
        <div
          role="radiogroup"
          aria-label={`Оценка: выберите от 1 до ${max}`}
          className="flex gap-0.5"
        >
          {Array.from({ length: max }, (_, i) => {
            const value = i + 1;
            const checked = value === rounded;
            return (
              <button
                key={i}
                type="button"
                role="radio"
                aria-checked={checked}
                aria-label={`${value} из ${max}`}
                className={cn(
                  "cursor-pointer rounded-sm transition-colors",
                  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1",
                )}
                onClick={() => onChange?.(value)}
                onKeyDown={(e) => {
                  if (e.key === "ArrowRight" && value < max) onChange?.(value + 1);
                  if (e.key === "ArrowLeft" && value > 1) onChange?.(value - 1);
                }}
              >
                <Star
                  aria-hidden="true"
                  className={cn(
                    sizeMap[size],
                    "transition-colors",
                    i < rounded
                      ? "fill-amber-500 text-amber-500"
                      : "fill-muted text-muted hover:text-amber-400",
                  )}
                />
              </button>
            );
          })}
        </div>
        {showValue && (
          <span className="ml-1 text-sm font-medium text-muted-foreground" aria-hidden="true">
            {rating.toFixed(1)}
          </span>
        )}
      </div>
    );
  }

  // Режим отображения — один семантический контейнер, без лишних tab-stops.
  return (
    <div className="flex items-center gap-1">
      <div
        role="img"
        aria-label={`Рейтинг ${rating.toFixed(1)} из ${max}`}
        className="flex gap-0.5"
      >
        {Array.from({ length: max }, (_, i) => (
          <Star
            key={i}
            aria-hidden="true"
            className={cn(
              sizeMap[size],
              "transition-colors",
              i < rounded ? "fill-amber-500 text-amber-500" : "fill-muted text-muted",
            )}
          />
        ))}
      </div>
      {showValue && (
        <span className="ml-1 text-sm font-medium text-muted-foreground" aria-hidden="true">
          {rating.toFixed(1)}
        </span>
      )}
    </div>
  );
}
