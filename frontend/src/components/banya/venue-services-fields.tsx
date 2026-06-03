"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { cn } from "@/lib/utils";
import {
  newVenueServiceLine,
  type VenueServiceFormLine,
} from "@/lib/types";
import { GripVertical, Plus, Trash2 } from "lucide-react";

// Должно совпадать с пределом валидации в venue-service
// (maxServiceDurationMin = 10080 = 7 дней).
const MAX_SERVICE_DURATION_MIN = 10080;

export function VenueServicesFields({
  value,
  onChange,
  disabled,
  className,
}: {
  value: VenueServiceFormLine[];
  onChange: (next: VenueServiceFormLine[]) => void;
  disabled?: boolean;
  className?: string;
}) {
  const [dragFrom, setDragFrom] = useState<number | null>(null);

  const addRow = () => {
    onChange([...value, newVenueServiceLine()]);
  };

  const updateRow = (index: number, patch: Partial<VenueServiceFormLine>) => {
    const next = value.map((row, i) =>
      i === index ? { ...row, ...patch } : row,
    );
    onChange(next);
  };

  const removeRow = (index: number) => {
    onChange(value.filter((_, i) => i !== index));
  };

  const reorder = (from: number, to: number) => {
    if (from === to || from < 0 || to < 0) return;
    if (from >= value.length || to >= value.length) return;
    const next = [...value];
    const [item] = next.splice(from, 1);
    next.splice(to, 0, item);
    onChange(next);
  };

  return (
    <Card className={cn("border-border", className)}>
      <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0 pb-4">
        <div className="space-y-1.5">
          <CardTitle className="text-lg">Услуги</CardTitle>
          <CardDescription>
            Добавьте услуги с ценами и продолжительностью
          </CardDescription>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="shrink-0 gap-1"
          onClick={addRow}
          disabled={disabled}
        >
          <Plus className="h-4 w-4" />
          Добавить
        </Button>
      </CardHeader>
      <CardContent className="space-y-3">
        {value.length === 0 ? (
          <p className="rounded-lg border border-dashed border-border py-8 text-center text-sm text-muted-foreground">
            Пока нет услуг — нажмите «Добавить», чтобы указать парные залы, сауны
            и т.п.
          </p>
        ) : null}
        {value.map((line, i) => {
          return (
            <div
              key={line.key}
              className={cn(
                "flex flex-col gap-2 rounded-lg border border-border bg-card p-3 sm:flex-row sm:items-center sm:gap-3",
                dragFrom === i && "opacity-60",
              )}
              onDragOver={(e) => {
                e.preventDefault();
                e.dataTransfer.dropEffect = "move";
              }}
              onDrop={(e) => {
                e.preventDefault();
                if (dragFrom === null) return;
                reorder(dragFrom, i);
                setDragFrom(null);
              }}
            >
              <button
                type="button"
                className="shrink-0 cursor-grab touch-none text-muted-foreground hover:text-foreground active:cursor-grabbing"
                draggable={!disabled}
                aria-label="Перетащить строку"
                onDragStart={(e) => {
                  if (disabled) {
                    e.preventDefault();
                    return;
                  }
                  setDragFrom(i);
                  e.dataTransfer.effectAllowed = "move";
                }}
                onDragEnd={() => setDragFrom(null)}
              >
                <GripVertical className="h-5 w-5" />
              </button>
              <Input
                className="min-w-0 flex-1"
                placeholder="Название услуги"
                value={line.name}
                disabled={disabled}
                onChange={(e) => updateRow(i, { name: e.target.value })}
              />
              <div className="flex flex-wrap items-center gap-2 sm:shrink-0">
                <Input
                  type="number"
                  min={0}
                  className="w-full sm:w-28"
                  placeholder="Цена, ₽"
                  value={line.price === 0 ? "" : line.price}
                  disabled={disabled}
                  onChange={(e) => {
                    const v = e.target.value;
                    updateRow(i, {
                      price: v === "" ? 0 : Math.max(0, Number(v) || 0),
                    });
                  }}
                />
                <div className="relative w-full sm:w-[140px]">
                  <Input
                    type="number"
                    min={1}
                    max={MAX_SERVICE_DURATION_MIN}
                    step={1}
                    inputMode="numeric"
                    className="w-full pr-12"
                    placeholder="Длительность"
                    aria-label="Длительность услуги, минут"
                    value={line.duration_min === 0 ? "" : line.duration_min}
                    disabled={disabled}
                    onChange={(e) => {
                      const v = e.target.value;
                      updateRow(i, {
                        duration_min:
                          v === ""
                            ? 0
                            : Math.min(
                                MAX_SERVICE_DURATION_MIN,
                                Math.max(0, Math.round(Number(v) || 0)),
                              ),
                      });
                    }}
                  />
                  <span className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-sm text-muted-foreground">
                    мин
                  </span>
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="shrink-0 text-muted-foreground hover:text-destructive"
                  disabled={disabled}
                  aria-label="Удалить услугу"
                  onClick={() => removeRow(i)}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            </div>
          );
        })}
      </CardContent>
    </Card>
  );
}
