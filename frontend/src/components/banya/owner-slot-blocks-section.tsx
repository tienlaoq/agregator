"use client";

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
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
import { Trash2 } from "lucide-react";
import {
  createOwnerSlotBlock,
  deleteOwnerSlotBlock,
  formatApiErrorMessage,
  getVenueBookingMode,
  listOwnerSlotBlocks,
} from "@/lib/api";
import type { VenueHall } from "@/lib/types";

function defaultDateRange(): { from: string; to: string } {
  const t = new Date();
  const from = t.toISOString().slice(0, 10);
  const end = new Date(t);
  end.setDate(end.getDate() + 30);
  const to = end.toISOString().slice(0, 10);
  return { from, to };
}

export function OwnerSlotBlocksSection({
  venueId,
  halls = [],
}: {
  venueId: string;
  halls?: VenueHall[];
}) {
  const qc = useQueryClient();
  const initialRange = useMemo(() => defaultDateRange(), []);
  const [dateFrom, setDateFrom] = useState(initialRange.from);
  const [dateTo, setDateTo] = useState(initialRange.to);

  const [formDate, setFormDate] = useState(initialRange.from);
  const [timeFrom, setTimeFrom] = useState("10:00");
  const [timeTo, setTimeTo] = useState("12:00");
  const [note, setNote] = useState("");
  const [formErr, setFormErr] = useState("");

  const { data: modeData } = useQuery({
    queryKey: ["venue-booking-mode", venueId],
    queryFn: () => getVenueBookingMode(venueId),
    enabled: Boolean(venueId),
  });
  const perHall = modeData?.booking_mode === "per_hall";
  // A per_hall venue with no halls at all behaves as whole (backend falls back
  // to a whole-venue block), so a hall is only required when halls exist.
  const requireHall = perHall && halls.length > 0;

  // When a hall is required, default to the first one. Unused otherwise.
  const [blockHall, setBlockHall] = useState("");
  // Adjust during render (React's documented alternative to a setState-in-effect):
  // requireHall implies halls.length > 0, so halls[0] is safe. Converges in one
  // extra render once blockHall matches an existing hall.
  if (requireHall && !halls.some((h) => h.id === blockHall)) {
    setBlockHall(halls[0].id);
  }

  const { data, isLoading, error } = useQuery({
    queryKey: ["owner-slot-blocks", venueId, dateFrom, dateTo],
    queryFn: () => listOwnerSlotBlocks(venueId, dateFrom, dateTo),
    enabled: Boolean(venueId),
  });

  const createMut = useMutation({
    mutationFn: () =>
      createOwnerSlotBlock(venueId, {
        date: formDate,
        time_from: timeFrom,
        time_to: timeTo,
        note: note.trim() || undefined,
        hall_ids: requireHall && blockHall ? [blockHall] : undefined,
      }),
    onSuccess: () => {
      setFormErr("");
      void qc.invalidateQueries({
        queryKey: ["owner-slot-blocks", venueId],
      });
    },
    onError: (e) => {
      setFormErr(formatApiErrorMessage(e, "Не удалось сохранить занятость"));
    },
  });

  const deleteMut = useMutation({
    mutationFn: (blockId: string) => deleteOwnerSlotBlock(venueId, blockId),
    onSuccess: () => {
      void qc.invalidateQueries({
        queryKey: ["owner-slot-blocks", venueId],
      });
    },
  });

  const blocks = data?.blocks ?? [];
  const listErr = error
    ? formatApiErrorMessage(error, "Не удалось загрузить список")
    : "";

  return (
    <Card className="border-border">
      <CardHeader className="pb-4">
        <CardTitle className="text-lg">Занятость вне сайта</CardTitle>
        <CardDescription>
          Отметьте время, уже проданное по телефону, в соцсетях или до
          подключения к агрегатору — тогда клиенты не смогут выбрать этот слот
          онлайн.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-end">
          <div className="space-y-2">
            <Label htmlFor="slot-range-from">Период с</Label>
            <Input
              id="slot-range-from"
              type="date"
              value={dateFrom}
              onChange={(e) => setDateFrom(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="slot-range-to">по</Label>
            <Input
              id="slot-range-to"
              type="date"
              value={dateTo}
              onChange={(e) => setDateTo(e.target.value)}
            />
          </div>
        </div>

        {listErr ? (
          <p className="text-sm text-destructive">{listErr}</p>
        ) : null}

        {isLoading ? (
          <p className="text-sm text-muted-foreground">Загрузка…</p>
        ) : blocks.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            В выбранном диапазоне нет ручных блокировок.
          </p>
        ) : (
          <ul className="divide-y rounded-md border border-border">
            {blocks.map((b) => (
              <li
                key={b.id}
                className="flex flex-col gap-2 p-3 sm:flex-row sm:items-center sm:justify-between"
              >
                <div>
                  <p className="font-medium text-foreground">
                    {b.date}{" "}
                    <span className="text-muted-foreground">
                      {b.time_from}–{b.time_to}
                    </span>
                  </p>
                  {b.note ? (
                    <p className="text-sm text-muted-foreground">{b.note}</p>
                  ) : null}
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="shrink-0 gap-1"
                  disabled={deleteMut.isPending}
                  onClick={() => deleteMut.mutate(b.id)}
                >
                  <Trash2 className="h-4 w-4" />
                  Убрать
                </Button>
              </li>
            ))}
          </ul>
        )}

        <div className="rounded-lg border border-dashed border-border p-4">
          <p className="mb-4 text-sm font-medium text-foreground">
            Добавить занятый интервал
          </p>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="slot-new-date">Дата</Label>
              <Input
                id="slot-new-date"
                type="date"
                value={formDate}
                onChange={(e) => setFormDate(e.target.value)}
              />
            </div>
            <div className="grid grid-cols-2 gap-2 sm:col-span-2 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="slot-new-from">С</Label>
                <Input
                  id="slot-new-from"
                  type="time"
                  value={timeFrom}
                  onChange={(e) => setTimeFrom(e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="slot-new-to">До</Label>
                <Input
                  id="slot-new-to"
                  type="time"
                  value={timeTo}
                  onChange={(e) => setTimeTo(e.target.value)}
                />
              </div>
            </div>
            {requireHall ? (
              <div className="space-y-2 sm:col-span-2">
                <Label>Зал</Label>
                <Select value={blockHall} onValueChange={setBlockHall}>
                  <SelectTrigger className="w-full sm:w-[220px]">
                    <SelectValue placeholder="Выберите зал" />
                  </SelectTrigger>
                  <SelectContent>
                    {halls.map((h) => (
                      <SelectItem key={h.id} value={h.id}>
                        {h.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            ) : null}
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="slot-new-note">Комментарий (необязательно)</Label>
              <Textarea
                id="slot-new-note"
                value={note}
                onChange={(e) => setNote(e.target.value)}
                placeholder="Например: бронь по телефону, Иванов"
                rows={2}
                className="resize-y"
              />
            </div>
          </div>
          {formErr ? (
            <p className="mt-3 text-sm text-destructive">{formErr}</p>
          ) : null}
          <Button
            type="button"
            className="mt-4"
            disabled={createMut.isPending || (requireHall && !blockHall)}
            onClick={() => createMut.mutate()}
          >
            {createMut.isPending ? "Сохранение…" : "Добавить занятость"}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
