"use client";

import { Check, X } from "lucide-react";
import { cn } from "@/lib/utils";
import {
  PASSWORD_RULES,
  evaluatePasswordStrength,
} from "@/lib/password";

/**
 * Индикатор надёжности пароля + чек-лист обязательных требований.
 * Показывается под полем ввода пароля на формах регистрации.
 * Ничего не блокирует сам по себе — родитель использует isPasswordValid().
 */
export function PasswordStrength({
  value,
  className,
}: {
  value: string;
  className?: string;
}) {
  if (!value) return null;

  const strength = evaluatePasswordStrength(value);
  const barColor =
    strength.level === "strong"
      ? "bg-green-500"
      : strength.level === "medium"
        ? "bg-amber-500"
        : "bg-destructive";
  const labelColor =
    strength.level === "strong"
      ? "text-green-600"
      : strength.level === "medium"
        ? "text-amber-600"
        : "text-destructive";

  return (
    <div className={cn("space-y-2", className)} aria-live="polite">
      <div className="flex items-center gap-2">
        <div className="flex h-1.5 flex-1 gap-1">
          {[1, 2, 3, 4].map((seg) => (
            <div
              key={seg}
              className={cn(
                "h-full flex-1 rounded-full transition-colors",
                seg <= strength.score ? barColor : "bg-muted",
              )}
            />
          ))}
        </div>
        {strength.label && (
          <span className={cn("w-16 shrink-0 text-right text-xs font-medium", labelColor)}>
            {strength.label}
          </span>
        )}
      </div>
      <ul className="space-y-1">
        {PASSWORD_RULES.map((rule) => {
          const ok = rule.test(value);
          return (
            <li
              key={rule.key}
              className={cn(
                "flex items-center gap-1.5 text-xs",
                ok ? "text-green-600" : "text-muted-foreground",
              )}
            >
              {ok ? (
                <Check className="h-3.5 w-3.5 shrink-0" />
              ) : (
                <X className="h-3.5 w-3.5 shrink-0" />
              )}
              {rule.label}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
