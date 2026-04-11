"use client";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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
      {VENUE_SOCIAL_FIELD_DEFS.map(({ key, label, placeholder }) => (
        <div key={key} className="space-y-2">
          <Label htmlFor={`social-${key}`}>{label}</Label>
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
