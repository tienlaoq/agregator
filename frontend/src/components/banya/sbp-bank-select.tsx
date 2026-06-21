"use client";

import { useQuery } from "@tanstack/react-query";
import { getSbpBanks } from "@/lib/api";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

/**
 * Bank picker for СБП payouts: the master/owner selects their bank by name and we
 * submit the member id. Avoids making them type a raw СБП identifier. The list
 * comes from the backend directory (/api/v1/sbp/banks) — currently a stub until
 * the payout provider is chosen.
 */
export function SbpBankSelect({
  value,
  onChange,
}: {
  value?: string;
  onChange: (value: string) => void;
}) {
  const { data: banks, isPending } = useQuery({
    queryKey: ["sbp-banks"],
    queryFn: getSbpBanks,
    staleTime: 1000 * 60 * 60,
  });

  return (
    <Select value={value || ""} onValueChange={onChange} disabled={isPending}>
      <SelectTrigger>
        <SelectValue
          placeholder={isPending ? "Загрузка банков…" : "Выберите банк"}
        />
      </SelectTrigger>
      <SelectContent>
        {banks?.map((b) => (
          <SelectItem key={b.id} value={b.id}>
            {b.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
