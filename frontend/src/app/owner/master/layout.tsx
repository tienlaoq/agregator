"use client";

import type { ReactNode } from "react";
import { MasterWorkspaceShell } from "@/components/banya/master-workspace-shell";

/**
 * Layout кабинета пар-мастера: общий сайдбар на всех разделах /owner/master.
 * Шапку даёт /owner/layout.tsx выше — здесь только рабочий shell.
 */
export default function MasterCabinetLayout({
  children,
}: {
  children: ReactNode;
}) {
  return <MasterWorkspaceShell>{children}</MasterWorkspaceShell>;
}
