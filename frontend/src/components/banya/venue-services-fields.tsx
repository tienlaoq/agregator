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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import {
  VENUE_SERVICE_DURATION_OPTIONS,
  newVenueServiceLine,
  type VenueServiceFormLine,
} from "@/lib/types";
import { GripVertical, Plus, Trash2 } from "lucide-react";

function durationOptionsFor(currentMin: number) {
  const base = [...VENUE_SERVICE_DURATION_OPTIONS];
  if (currentMin > 0 && !base.some((o) => o.value === currentMin)) {
    base.push({ value: currentMin, label: `${currentMin} мин` });
    base.sort((a, b) => a.value - b.value);
  }
  return base;
}

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
          const opts = durationOptionsFor(line.duration_min);
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
                <Select
                  value={String(line.duration_min)}
                  disabled={disabled}
                  onValueChange={(v) =>
                    updateRow(i, { duration_min: Number(v) || 0 })
                  }
                >
                  <SelectTrigger className="w-full sm:w-[140px]">
                    <SelectValue placeholder="Длительность" />
                  </SelectTrigger>
                  <SelectContent>
                    {opts.map((o) => (
                      <SelectItem key={o.value} value={String(o.value)}>
                        {o.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
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
