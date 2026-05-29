import { AppLayout } from "@/app/app-layout"

/**
 * Публичный layout — применяется ко всем страницам в группе (public):
 * главная, каталог, карточки venue/master, личный кабинет, статичные страницы.
 * /admin, /owner, /auth получают собственные layout-ы без шапки/футера/чата.
 */
export default function PublicLayout({ children }: { children: React.ReactNode }) {
  return <AppLayout>{children}</AppLayout>
}
