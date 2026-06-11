"use client";

import { useEffect, useMemo } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Activity, AlertTriangle, CheckCircle2, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useAuthStore } from "@/store/auth";

type RouteStat = {
  key: string;
  method: string;
  route: string;
  total: number;
  errors4xx5xx: number;
  avgMs: number | null;
};

function parseLabels(raw: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const part of raw.split(",")) {
    const m = part.match(/^\s*([^=]+)="(.*)"\s*$/);
    if (!m) continue;
    out[m[1]] = m[2];
  }
  return out;
}

function parsePrometheusMetrics(raw: string): {
  routeStats: RouteStat[];
  supportSuccess: number;
  supportError: number;
} {
  const reqMap = new Map<string, RouteStat>();
  const durMap = new Map<string, { sum: number; count: number }>();
  let supportSuccess = 0;
  let supportError = 0;

  for (const line of raw.split("\n")) {
    if (!line || line.startsWith("#")) continue;

    let m = line.match(/^api_gateway_http_requests_total\{(.+)\}\s+([0-9.]+)$/);
    if (m) {
      const labels = parseLabels(m[1]);
      const method = labels.method || "UNKNOWN";
      const route = labels.route || "/";
      const statusClass = labels.status_class || "2xx";
      const val = Number(m[2]) || 0;
      const key = `${method}|${route}`;
      const prev = reqMap.get(key) ?? {
        key,
        method,
        route,
        total: 0,
        errors4xx5xx: 0,
        avgMs: null,
      };
      prev.total += val;
      if (statusClass === "4xx" || statusClass === "5xx") prev.errors4xx5xx += val;
      reqMap.set(key, prev);
      continue;
    }

    m = line.match(/^api_gateway_http_request_duration_seconds_(sum|count)\{(.+)\}\s+([0-9.]+)$/);
    if (m) {
      const kind = m[1];
      const labels = parseLabels(m[2]);
      const method = labels.method || "UNKNOWN";
      const route = labels.route || "/";
      const key = `${method}|${route}`;
      const prev = durMap.get(key) ?? { sum: 0, count: 0 };
      const val = Number(m[3]) || 0;
      if (kind === "sum") prev.sum += val;
      if (kind === "count") prev.count += val;
      durMap.set(key, prev);
      continue;
    }

    m = line.match(/^api_gateway_support_webhook_deliveries_total\{(.+)\}\s+([0-9.]+)$/);
    if (m) {
      const labels = parseLabels(m[1]);
      const result = labels.result || "";
      const val = Number(m[2]) || 0;
      if (result === "success") supportSuccess += val;
      if (result === "error") supportError += val;
    }
  }

  const routeStats = Array.from(reqMap.values()).map((r) => {
    const d = durMap.get(r.key);
    const avgMs = d && d.count > 0 ? (d.sum / d.count) * 1000 : null;
    return { ...r, avgMs };
  });
  routeStats.sort((a, b) => b.total - a.total);

  return { routeStats, supportSuccess, supportError };
}

export default function AdminMetricsPage() {
  const router = useRouter();
  const { token, user, hydrated } = useAuthStore();

  useEffect(() => {
    if (hydrated && (!token || user?.role !== "admin")) {
      router.push("/");
    }
  }, [hydrated, token, user, router]);

  const metricsQuery = useQuery({
    queryKey: ["admin-metrics", token],
    enabled: !!token && user?.role === "admin",
    queryFn: async () => {
      // Prometheus exposition — text, не JSON, поэтому не через fetchAPI.
      // Эндпоинт за RequireRole(admin): публичный /metrics с гейтвея убран
      // (живёт на внутреннем METRICS_ADDR-листенере для Prometheus).
      const base = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
      const res = await fetch(`${base}/api/v1/admin/metrics`, {
        cache: "no-store",
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      if (!res.ok) throw new Error(`metrics status ${res.status}`);
      return res.text();
    },
    refetchInterval: 15000,
  });

  const parsed = useMemo(
    () =>
      parsePrometheusMetrics(metricsQuery.data || ""),
    [metricsQuery.data],
  );

  const totalRequests = parsed.routeStats.reduce((acc, s) => acc + s.total, 0);
  const totalErrors = parsed.routeStats.reduce((acc, s) => acc + s.errors4xx5xx, 0);
  const errorRate = totalRequests > 0 ? (totalErrors / totalRequests) * 100 : 0;

  if (!hydrated || !token || user?.role !== "admin") return null;

  return (
    <div className="mx-auto max-w-6xl px-4 py-8">
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-bold">Метрики платформы</h1>
        <div className="flex gap-2">
          <Button
            variant="outline"
            disabled={!metricsQuery.data}
            onClick={() => {
              if (!metricsQuery.data) return;
              const url = URL.createObjectURL(
                new Blob([metricsQuery.data], { type: "text/plain" }),
              );
              window.open(url, "_blank");
              URL.revokeObjectURL(url);
            }}
          >
            Raw /metrics
          </Button>
          <Button variant="outline" onClick={() => metricsQuery.refetch()} disabled={metricsQuery.isFetching}>
            <RefreshCw className={`mr-2 h-4 w-4 ${metricsQuery.isFetching ? "animate-spin" : ""}`} />
            Обновить
          </Button>
        </div>
      </div>

      {metricsQuery.isError ? (
        <Card className="mb-6 border-destructive/40">
          <CardContent className="pt-6 text-destructive">
            Не удалось загрузить метрики. Проверьте доступность `api-gateway` и endpoint `/metrics`.
          </CardContent>
        </Card>
      ) : null}

      <div className="mb-6 grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
              <Activity className="h-4 w-4" /> Запросы (всего)
            </CardTitle>
          </CardHeader>
          <CardContent className="text-2xl font-semibold">{Math.round(totalRequests)}</CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
              <AlertTriangle className="h-4 w-4" /> Ошибки 4xx/5xx
            </CardTitle>
          </CardHeader>
          <CardContent className="text-2xl font-semibold">{Math.round(totalErrors)}</CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Error rate</CardTitle>
          </CardHeader>
          <CardContent className="text-2xl font-semibold">{errorRate.toFixed(2)}%</CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Support webhook</CardTitle>
          </CardHeader>
          <CardContent className="space-y-1 text-sm">
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">success</span>
              <span className="inline-flex items-center font-semibold text-emerald-600">
                <CheckCircle2 className="mr-1 h-4 w-4" />
                {Math.round(parsed.supportSuccess)}
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">error</span>
              <span className="inline-flex items-center font-semibold text-destructive">
                <AlertTriangle className="mr-1 h-4 w-4" />
                {Math.round(parsed.supportError)}
              </span>
            </div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Топ роутов по трафику</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left text-muted-foreground">
                  <th className="py-2 pr-2">Method</th>
                  <th className="py-2 pr-2">Route</th>
                  <th className="py-2 pr-2 text-right">Requests</th>
                  <th className="py-2 pr-2 text-right">4xx/5xx</th>
                  <th className="py-2 pr-0 text-right">Avg latency</th>
                </tr>
              </thead>
              <tbody>
                {parsed.routeStats.slice(0, 20).map((s) => (
                  <tr key={s.key} className="border-b last:border-0">
                    <td className="py-2 pr-2 font-mono">{s.method}</td>
                    <td className="py-2 pr-2 font-mono text-xs">{s.route}</td>
                    <td className="py-2 pr-2 text-right">{Math.round(s.total)}</td>
                    <td className="py-2 pr-2 text-right">{Math.round(s.errors4xx5xx)}</td>
                    <td className="py-2 pr-0 text-right">
                      {s.avgMs == null ? "—" : `${s.avgMs.toFixed(1)} ms`}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

