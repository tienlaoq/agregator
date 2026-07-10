"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Loader2 } from "lucide-react";
import { listMyMasterClients, ApiError } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import {
  clientDisplayName,
  clientSegmentLabel,
  clientSegmentVariant,
  formatClientDate,
  formatKopecks,
} from "@/lib/master-clients";

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).slice(0, 2);
  return parts.map((p) => p[0]?.toUpperCase() ?? "").join("") || "К";
}

export default function MasterClientsPage() {
  const router = useRouter();
  const { token, user, hydrated } = useAuthStore();
  const enabled = !!token && user?.role === "master";

  const [segment, setSegment] = useState("all");
  const [sort, setSort] = useState("last_visit");

  useEffect(() => {
    if (hydrated && (!token || user?.role !== "master")) {
      router.push("/auth/login");
    }
  }, [hydrated, token, user, router]);

  const { data, isLoading, error } = useQuery({
    queryKey: ["my-master-clients", segment, sort],
    queryFn: () =>
      listMyMasterClients({
        segment: segment === "all" ? undefined : segment,
        sort,
      }),
    enabled,
    retry: false,
  });

  if (!hydrated || !enabled) return null;

  const noProfile = error instanceof ApiError && error.status === 404;
  const clients = data?.clients ?? [];

  return (
    <div className="container mx-auto max-w-4xl px-4 py-10">
        <div className="mb-6">
          <h1 className="text-2xl font-bold">Клиенты</h1>
          <p className="text-muted-foreground">
            База клиентов из ваших заявок: сегменты, визиты и суммарный доход.
          </p>
        </div>

        <div className="mb-4 flex flex-wrap items-end gap-4">
          <div className="grid gap-1.5">
            <Label className="text-xs text-muted-foreground">Сегмент</Label>
            <Select value={segment} onValueChange={setSegment}>
              <SelectTrigger className="w-44">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Все клиенты</SelectItem>
                <SelectItem value="new">Новые</SelectItem>
                <SelectItem value="regular">Постоянные</SelectItem>
                <SelectItem value="at_risk">Спящие</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label className="text-xs text-muted-foreground">Сортировка</Label>
            <Select value={sort} onValueChange={setSort}>
              <SelectTrigger className="w-52">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="last_visit">По последнему визиту</SelectItem>
                <SelectItem value="ltv">По сумме (LTV)</SelectItem>
                <SelectItem value="visits">По числу визитов</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {typeof data?.total === "number" && (
            <div className="ml-auto text-sm text-muted-foreground">
              Всего: {data.total}
            </div>
          )}
        </div>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Список клиентов</CardTitle>
            <CardDescription>
              Обновляется по мере поступления и завершения заявок.
            </CardDescription>
          </CardHeader>
          <CardContent className="overflow-x-auto">
            {isLoading ? (
              <div className="flex items-center justify-center py-12 text-muted-foreground">
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Загрузка…
              </div>
            ) : noProfile ? (
              <p className="py-10 text-center text-sm text-muted-foreground">
                Сначала опубликуйте профиль мастера — клиенты появятся после
                первых заявок.
              </p>
            ) : error ? (
              <p className="py-10 text-center text-sm text-destructive">
                Не удалось загрузить клиентов.
              </p>
            ) : clients.length === 0 ? (
              <p className="py-10 text-center text-sm text-muted-foreground">
                {segment === "all"
                  ? "Пока нет клиентов. Они появятся, когда кто-то забронирует ваши услуги."
                  : "Нет клиентов в этом сегменте."}
              </p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Клиент</TableHead>
                    <TableHead>Сегменты</TableHead>
                    <TableHead className="text-center">Визиты</TableHead>
                    <TableHead>Последний визит</TableHead>
                    <TableHead className="text-right">Доход</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {clients.map((c) => {
                    const name = clientDisplayName(c);
                    return (
                      <TableRow key={c.user_id}>
                        <TableCell>
                          <div className="flex items-center gap-3">
                            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-medium text-primary">
                              {initials(name)}
                            </div>
                            <span className="font-medium">{name}</span>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex flex-wrap gap-1">
                            {c.segments.length === 0 ? (
                              <span className="text-sm text-muted-foreground">
                                —
                              </span>
                            ) : (
                              c.segments.map((s) => (
                                <Badge key={s} variant={clientSegmentVariant(s)}>
                                  {clientSegmentLabel(s)}
                                </Badge>
                              ))
                            )}
                          </div>
                        </TableCell>
                        <TableCell className="text-center text-sm">
                          {c.visits_count}
                          <span className="text-muted-foreground">
                            {" "}
                            / {c.bookings_count}
                          </span>
                        </TableCell>
                        <TableCell className="text-sm">
                          {formatClientDate(c.last_visit_at)}
                        </TableCell>
                        <TableCell className="text-right font-medium">
                          {formatKopecks(c.total_spent)}
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>
  );
}
