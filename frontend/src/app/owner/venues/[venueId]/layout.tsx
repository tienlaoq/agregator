"use client";

import type { ReactNode } from "react";
import { useParams } from "next/navigation";
import { OwnerWorkspaceShell } from "@/components/banya/owner-workspace-shell";

export default function VenueWorkspaceLayout({
  children,
}: {
  children: ReactNode;
}) {
  const params = useParams<{ venueId: string }>();
  return (
    <OwnerWorkspaceShell venueId={params.venueId}>
      {children}
    </OwnerWorkspaceShell>
  );
}
