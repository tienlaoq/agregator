import Link from "next/link"

/**
 * Каркас юридической страницы-заглушки: хлебные крошки + заголовок + текст.
 * Реальный юридический текст вставляется вместо `children` (готовит юрист).
 */
export function LegalPlaceholder({
  title,
  children,
  className,
}: {
  title: string
  children: React.ReactNode
  className?: string
}) {
  return (
    <div className="container mx-auto max-w-3xl px-4 py-12">
      <nav className="mb-6 text-sm text-muted-foreground">
        <Link href="/" className="hover:text-foreground">
          Главная
        </Link>
        {" / "}
        <span className="text-foreground">{title}</span>
      </nav>
      <h1 className="mb-6 text-3xl font-bold text-foreground">{title}</h1>
      <div
        className={`max-w-none space-y-4 text-muted-foreground leading-relaxed ${className ?? ""}`}
      >
        {children}
      </div>
    </div>
  )
}
