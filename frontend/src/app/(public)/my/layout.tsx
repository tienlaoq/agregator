import { UserWorkspaceShell } from "@/components/banya/user-workspace-shell";

/**
 * Layout личного кабинета пользователя. Оборачивает все страницы `/my/*`
 * в оболочку с боковым меню; сам находится внутри публичного AppLayout
 * (шапка + футер + чат).
 */
export default function MyLayout({ children }: { children: React.ReactNode }) {
  return <UserWorkspaceShell>{children}</UserWorkspaceShell>;
}
