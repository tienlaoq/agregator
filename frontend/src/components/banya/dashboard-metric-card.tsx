import { Card, CardContent } from "@/components/ui/card";
import type { LucideIcon } from "lucide-react";

/** Summary metric tile shared by the owner and master dashboards. */
export function MetricCard({
  title,
  value,
  icon: Icon,
}: {
  title: string;
  value: string;
  icon: LucideIcon;
}) {
  return (
    <Card className="border-border">
      <CardContent className="flex items-center gap-4 p-6">
        <div className="flex h-12 w-12 items-center justify-center rounded-lg bg-primary/10">
          <Icon className="h-6 w-6 text-primary" />
        </div>
        <div>
          <p className="text-sm text-muted-foreground">{title}</p>
          <p className="text-2xl font-bold text-card-foreground">{value}</p>
        </div>
      </CardContent>
    </Card>
  );
}
