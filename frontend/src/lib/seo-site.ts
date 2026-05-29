/** Публичный origin сайта (без хвоста /). Для canonical, sitemap, Open Graph. */
export function siteUrl(): string {
  const raw = (process.env.NEXT_PUBLIC_SITE_URL || "http://localhost:3000").trim()
  return raw.replace(/\/+$/, "")
}
