"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Zap, Hand, Ban, Plus, Trash2, Loader2, Lock } from "lucide-react";
import {
  getMyMasterProfile,
  listMasterSlotBlocks,
  createMasterSlotBlock,
  deleteMasterSlotBlock,
  patchMyMasterProfile,
  formatApiErrorMessage,
} from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import {
  DOW_LABELS,
  parseWorkingHours,
  mergeHoursIntoAvailability,
  toLocalISODate,
  dowIndex,
  hasBlockOn,
  blockIntervalLabel,
  formatBlockDate,
  type WorkingHoursDay,
} from "@/lib/master-schedule";
import type { MasterSlotBlock } from "@/lib/types";

const blockSchema = z
  .object({
    date: z.string().min(1, "Укажите дату"),
    whole_day: z.boolean(),
    time_from: z.string().optional(),
    time_to: z.string().optional(),
    note: z.string().max(200, "Не длиннее 200 символов").optional(),
  })
  .refine((v) => v.whole_day || (!!v.time_from && !!v.time_to), {
    message: "Укажите начало и конец интервала",
    path: ["time_from"],
  })
  .refine(
    (v) => v.whole_day || !v.time_from || !v.time_to || v.time_to > v.time_from,
    { message: "Конец должен быть позже начала", path: ["time_to"] },
  );

type BlockForm = z.infer<typeof blockSchema>;

export default function MasterSchedulePage() {
  const router = useRouter();
  const qc = useQueryClient();
  const { token, user, hydrated } = useAuthStore();
  const enabled = !!token && user?.role === "master";

  useEffect(() => {
    if (hydrated && (!token || user?.role !== "master")) {
      router.push("/auth/login");
    }
  }, [hydrated, token, user, router]);

  const profileQ = useQuery({
    queryKey: ["my-master-profile"],
    queryFn: getMyMasterProfile,
    enabled,
    retry: false,
  });
  const blocksQ = useQuery({
    queryKey: ["my-master-slot-blocks"],
    queryFn: listMasterSlotBlocks,
    enabled: enabled && profileQ.data != null,
    retry: false,
  });

  const [hours, setHours] = useState<WorkingHoursDay[] | null>(null);
  useEffect(() => {
    if (profileQ.data) setHours(parseWorkingHours(profileQ.data.availability_json));
  }, [profileQ.data]);

  const savedHoursJSON = useMemo(
    () => JSON.stringify(parseWorkingHours(profileQ.data?.availability_json)),
    [profileQ.data],
  );
  const hoursDirty = hours != null && JSON.stringify(hours) !== savedHoursJSON;

  const saveHours = useMutation({
    mutationFn: (h: WorkingHoursDay[]) =>
      patchMyMasterProfile({
        availability_json: mergeHoursIntoAvailability(
          profileQ.data?.availability_json,
          h,
        ),
      }),
    onSuccess: () => {
      toast.success("Часы работы сохранены");
      qc.invalidateQueries({ queryKey: ["my-master-profile"] });
    },
    onError: (e) =>
      toast.error(formatApiErrorMessage(e, "Не удалось сохранить часы")),
  });

  const createBlock = useMutation({
    mutationFn: createMasterSlotBlock,
    onSuccess: () => {
      toast.success("Блокировка добавлена");
      qc.invalidateQueries({ queryKey: ["my-master-slot-blocks"] });
      reset({ date: "", whole_day: true, time_from: "", time_to: "", note: "" });
    },
    onError: (e) =>
      toast.error(formatApiErrorMessage(e, "Не удалось добавить блокировку")),
  });

  const deleteBlock = useMutation({
    mutationFn: deleteMasterSlotBlock,
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: ["my-master-slot-blocks"] }),
    onError: (e) =>
      toast.error(formatApiErrorMessage(e, "Не удалось удалить блокировку")),
  });

  const {
    register,
    handleSubmit,
    watch,
    reset,
    formState: { errors },
  } = useForm<BlockForm>({
    resolver: zodResolver(blockSchema),
    defaultValues: { date: "", whole_day: true, time_from: "", time_to: "", note: "" },
  });
  const wholeDay = watch("whole_day");

  const onAddBlock = handleSubmit((v) => {
    createBlock.mutate({
      date: v.date,
      time_from: v.whole_day ? "" : v.time_from,
      time_to: v.whole_day ? "" : v.time_to,
      note: v.note?.trim() || "",
    });
  });

  const blocks = useMemo(() => blocksQ.data?.blocks ?? [], [blocksQ.data]);
  const preview = useMemo(() => buildPreview(hours, blocks), [hours, blocks]);

  if (!hydrated || !enabled) return null;

  const noProfile = profileQ.isSuccess && profileQ.data == null;

  return (
    <div className="container mx-auto max-w-3xl px-4 py-10">
      <h1 className="mb-1 text-2xl font-bold">Расписание</h1>
      <p className="mb-6 text-muted-foreground">
        Часы работы, блокировки и режим приёма заявок.
      </p>

      {noProfile ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            Сначала создайте профиль мастера — расписание появится после этого.
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-5">
          {/* Booking mode */}
          <Card>
            <CardContent className="pt-6">
              <div className="mb-3 text-sm font-medium">Приём заявок</div>
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="rounded-lg border-2 border-primary/60 p-3">
                  <div className="flex items-center gap-2 font-medium">
                    <Hand className="h-4 w-4 text-primary" />
                    Ручное подтверждение
                  </div>
                  <p className="mt-1 text-sm text-muted-foreground">
                    Вы сами принимаете каждую заявку. Бесплатно.
                  </p>
                </div>
                <div className="rounded-lg border border-border p-3 opacity-80">
                  <div className="flex items-center gap-2 font-medium">
                    <Zap className="h-4 w-4 text-violet-500" />
                    Онлайн-запись
                    <span className="inline-flex items-center gap-1 rounded-full bg-violet-500/10 px-1.5 text-[11px] text-violet-600 dark:text-violet-300">
                      <Lock className="h-3 w-3" /> Про
                    </span>
                  </div>
                  <p className="mt-1 text-sm text-muted-foreground">
                    Клиент бронирует свободный слот сразу, без подтверждения.
                  </p>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Working hours */}
          <Card>
            <CardContent className="pt-6">
              <div className="mb-3 flex items-center justify-between">
                <div className="text-sm font-medium">Часы работы</div>
                {hoursDirty && (
                  <Button
                    size="sm"
                    onClick={() => hours && saveHours.mutate(hours)}
                    disabled={saveHours.isPending}
                  >
                    {saveHours.isPending && (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    )}
                    Сохранить
                  </Button>
                )}
              </div>
              {hours == null ? (
                <div className="h-40 animate-pulse rounded-lg bg-muted" />
              ) : (
                <div className="divide-y divide-border">
                  {hours.map((d, i) => (
                    <div key={i} className="flex items-center gap-3 py-2.5">
                      <button
                        type="button"
                        role="switch"
                        aria-checked={d.on}
                        aria-label={DOW_LABELS[i]}
                        onClick={() =>
                          setHours((prev) =>
                            prev
                              ? prev.map((x, j) =>
                                  j === i ? { ...x, on: !x.on } : x,
                                )
                              : prev,
                          )
                        }
                        className={`relative h-5 w-9 shrink-0 rounded-full transition-colors ${
                          d.on ? "bg-emerald-500" : "bg-muted-foreground/40"
                        }`}
                      >
                        <span
                          className={`absolute top-0.5 h-4 w-4 rounded-full bg-white transition-all ${
                            d.on ? "left-[18px]" : "left-0.5"
                          }`}
                        />
                      </button>
                      <span className="w-8 text-sm font-medium">
                        {DOW_LABELS[i]}
                      </span>
                      {d.on ? (
                        <div className="flex items-center gap-2 text-sm">
                          <Input
                            type="time"
                            value={d.from}
                            onChange={(e) =>
                              setHours((prev) =>
                                prev
                                  ? prev.map((x, j) =>
                                      j === i ? { ...x, from: e.target.value } : x,
                                    )
                                  : prev,
                              )
                            }
                            className="w-28"
                          />
                          <span className="text-muted-foreground">–</span>
                          <Input
                            type="time"
                            value={d.to}
                            onChange={(e) =>
                              setHours((prev) =>
                                prev
                                  ? prev.map((x, j) =>
                                      j === i ? { ...x, to: e.target.value } : x,
                                    )
                                  : prev,
                              )
                            }
                            className="w-28"
                          />
                        </div>
                      ) : (
                        <span className="text-sm text-muted-foreground">
                          Выходной
                        </span>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>

          {/* Blockings */}
          <Card>
            <CardContent className="pt-6">
              <div className="mb-1 text-sm font-medium">Блокировки</div>
              <p className="mb-4 text-sm text-muted-foreground">
                Отпуск, занятые дни или разовые перерывы — в эти интервалы заявки
                не принимаются.
              </p>

              <form
                onSubmit={onAddBlock}
                className="mb-4 grid gap-3 rounded-lg border border-border p-3 sm:grid-cols-2"
              >
                <div className="grid gap-1.5">
                  <Label className="text-xs text-muted-foreground">Дата</Label>
                  <Input
                    type="date"
                    min={toLocalISODate(new Date())}
                    {...register("date")}
                  />
                  {errors.date && (
                    <p className="text-xs text-destructive">
                      {errors.date.message}
                    </p>
                  )}
                </div>
                <div className="grid gap-1.5">
                  <Label className="text-xs text-muted-foreground">Заметка</Label>
                  <Input placeholder="Отпуск, занят…" {...register("note")} />
                  {errors.note && (
                    <p className="text-xs text-destructive">
                      {errors.note.message}
                    </p>
                  )}
                </div>
                <label className="flex items-center gap-2 text-sm sm:col-span-2">
                  <input type="checkbox" {...register("whole_day")} /> Весь день
                </label>
                {!wholeDay && (
                  <>
                    <div className="grid gap-1.5">
                      <Label className="text-xs text-muted-foreground">С</Label>
                      <Input type="time" {...register("time_from")} />
                      {errors.time_from && (
                        <p className="text-xs text-destructive">
                          {errors.time_from.message}
                        </p>
                      )}
                    </div>
                    <div className="grid gap-1.5">
                      <Label className="text-xs text-muted-foreground">До</Label>
                      <Input type="time" {...register("time_to")} />
                      {errors.time_to && (
                        <p className="text-xs text-destructive">
                          {errors.time_to.message}
                        </p>
                      )}
                    </div>
                  </>
                )}
                <div className="sm:col-span-2">
                  <Button type="submit" size="sm" disabled={createBlock.isPending}>
                    {createBlock.isPending ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <Plus className="mr-2 h-4 w-4" />
                    )}
                    Добавить блокировку
                  </Button>
                </div>
              </form>

              {blocksQ.isLoading ? (
                <div className="h-16 animate-pulse rounded-lg bg-muted" />
              ) : blocks.length === 0 ? (
                <p className="py-2 text-sm text-muted-foreground">
                  Блокировок нет.
                </p>
              ) : (
                <div className="divide-y divide-border">
                  {blocks.map((b) => (
                    <div key={b.id} className="flex items-center gap-3 py-2.5">
                      <Ban className="h-4 w-4 shrink-0 text-amber-500" />
                      <div className="flex-1">
                        <div className="text-sm font-medium">
                          {formatBlockDate(b.date)}
                        </div>
                        <div className="text-xs text-muted-foreground">
                          {blockIntervalLabel(b)}
                          {b.note ? ` · ${b.note}` : ""}
                        </div>
                      </div>
                      <button
                        type="button"
                        aria-label="Удалить блокировку"
                        onClick={() => deleteBlock.mutate(b.id)}
                        disabled={deleteBlock.isPending}
                        className="p-1 text-muted-foreground hover:text-destructive"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>

          {/* Week preview */}
          <Card>
            <CardContent className="pt-6">
              <div className="mb-1 text-sm font-medium">Превью недели</div>
              <p className="mb-4 text-sm text-muted-foreground">
                Как клиент видит вашу доступность на ближайшие 7 дней.
              </p>
              <div className="grid grid-cols-7 items-end gap-1.5">
                {preview.map((p) => (
                  <div key={p.iso} className="flex flex-col items-center gap-1.5">
                    <span
                      className={`text-[10px] ${
                        p.state === "block"
                          ? "text-amber-600"
                          : "text-muted-foreground"
                      }`}
                    >
                      {p.label}
                    </span>
                    <div
                      className={`w-full max-w-[34px] rounded ${p.barClass}`}
                      style={{ height: p.on ? 46 : 16 }}
                    />
                    <span className="text-[11px] font-medium">{p.dow}</span>
                    <span className="text-[10px] text-muted-foreground">
                      {p.day}
                    </span>
                  </div>
                ))}
              </div>
              <div className="mt-3 flex gap-4 text-[11px] text-muted-foreground">
                <span className="flex items-center gap-1.5">
                  <span className="h-2.5 w-2.5 rounded-sm bg-emerald-500" />
                  свободно
                </span>
                <span className="flex items-center gap-1.5">
                  <span className="h-2.5 w-2.5 rounded-sm bg-amber-500" />
                  блокировка
                </span>
                <span className="flex items-center gap-1.5">
                  <span className="h-2.5 w-2.5 rounded-sm bg-muted-foreground/40" />
                  выходной
                </span>
              </div>
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  );
}

type PreviewCol = {
  iso: string;
  dow: string;
  day: number;
  on: boolean;
  state: "free" | "block" | "off";
  label: string;
  barClass: string;
};

function buildPreview(
  hours: WorkingHoursDay[] | null,
  blocks: MasterSlotBlock[],
): PreviewCol[] {
  const days = hours ?? [];
  const out: PreviewCol[] = [];
  const today = new Date();
  for (let k = 0; k < 7; k++) {
    const d = new Date(today);
    d.setDate(today.getDate() + k);
    const iso = toLocalISODate(d);
    const idx = dowIndex(d);
    const on = days[idx]?.on ?? false;
    const blocked = hasBlockOn(blocks, iso);
    let state: PreviewCol["state"] = "off";
    let barClass = "bg-muted-foreground/40";
    let label = "вых";
    if (on && !blocked) {
      state = "free";
      barClass = "bg-emerald-500";
      label = "своб";
    } else if (on && blocked) {
      state = "block";
      barClass = "bg-amber-500";
      label = "блок";
    }
    out.push({
      iso,
      dow: DOW_LABELS[idx],
      day: d.getDate(),
      on,
      state,
      label,
      barClass,
    });
  }
  return out;
}
