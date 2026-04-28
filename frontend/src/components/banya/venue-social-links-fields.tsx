"use client";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { CircleAlert } from "lucide-react";
import {
  VENUE_SOCIAL_FIELD_DEFS,
  type VenueSocialLinkKey,
  type VenueSocialLinks,
} from "@/lib/types";

export function VenueSocialLinksFields({
  value,
  onChange,
}: {
  value: VenueSocialLinks;
  onChange: (next: VenueSocialLinks) => void;
}) {
  const set = (key: VenueSocialLinkKey, v: string) => {
    onChange({ ...value, [key]: v });
  };

  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {VENUE_SOCIAL_FIELD_DEFS.map(({ key, label, placeholder, hint }) => (
        <div key={key} className="space-y-2">
          <div className="flex items-center gap-1.5">
            <Label htmlFor={`social-${key}`}>{label}</Label>
            {hint ? (
              <>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      type="button"
                      className="hidden text-muted-foreground transition-colors hover:text-foreground sm:inline-flex"
                      aria-label={hint}
                      title={hint}
                    >
                      <CircleAlert className="h-4 w-4" />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent className="max-w-[280px] text-left" sideOffset={8}>
                    {hint}
                  </TooltipContent>
                </Tooltip>
                <Popover>
                  <PopoverTrigger asChild>
                    <button
                      type="button"
                      className="inline-flex text-muted-foreground transition-colors hover:text-foreground sm:hidden"
                      aria-label={hint}
                      title={hint}
                    >
                      <CircleAlert className="h-4 w-4" />
                    </button>
                  </PopoverTrigger>
                  <PopoverContent className="w-72 text-xs leading-relaxed" align="start">
                    {hint}
                  </PopoverContent>
                </Popover>
              </>
            ) : null}
          </div>
          <Input
            id={`social-${key}`}
            type="url"
            inputMode="url"
            autoComplete="off"
            placeholder={placeholder}
            value={value[key]}
            onChange={(e) => set(key, e.target.value)}
          />
        </div>
      ))}
    </div>
  );
}
