import type { MetadataRoute } from "next"
import { siteUrl } from "@/lib/seo-site"

export default function robots(): MetadataRoute.Robots {
  const base = siteUrl()
  return {
    rules: [
      {
        userAgent: "*",
        allow: "/",
        disallow: [
          // Личный кабинет, админка, регистрация
          "/admin",
          "/admin/",
          "/owner",
          "/owner/",
          "/my",
          "/my/",
          "/auth",
          "/auth/",
          // Фасетные параметры — дублирующий контент, краулер не должен индексировать.
          // Формат: /*?param= блокирует любые URL с этим параметром.
          "/*?q=",
          "/*?city=",
          "/*?price_min=",
          "/*?price_max=",
          "/*?type=",
          "/*?sort=",
          "/*?page=",
          // API routes
          "/api/",
        ],
      },
    ],
    sitemap: `${base}/sitemap.xml`,
    host: new URL(base).host,
  }
}
