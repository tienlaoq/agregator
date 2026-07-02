import type { LucideIcon } from "lucide-react";

export function WorkspacePlaceholder({
  icon: Icon,
  title,
  description,
}: {
  icon: LucideIcon;
  title: string;
  description: string;
}) {
  return (
    <div className="p-4 md:p-6">
      <h1 className="mb-5 text-2xl font-bold text-foreground">{title}</h1>
      <div className="flex flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-border bg-muted/20 px-6 py-16 text-center">
        <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-muted text-muted-foreground">
          <Icon className="h-6 w-6" />
        </div>
        <p className="max-w-md text-sm text-muted-foreground">{description}</p>
        <span className="rounded-full bg-muted px-3 py-1 text-xs text-muted-foreground">
          Скоро
        </span>
      </div>
    </div>
  );
}
