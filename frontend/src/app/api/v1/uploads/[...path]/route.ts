import { type NextRequest, NextResponse } from "next/server";

/**
 * GET /api/v1/uploads/<path...>
 *
 * Проксирует публичные медиа (фото площадок, аватары мастеров) с API-gateway.
 *
 * Зачем нужен этот route, а не прямая ссылка на gateway:
 *   - venueMediaUrl() отдаёт относительный путь (/api/v1/uploads/...), чтобы
 *     браузер и оптимизатор next/image ходили на свой же origin.
 *   - Оптимизатор next/image работает server-side ВНУТРИ контейнера фронтенда.
 *     Там `localhost:8080` — это сам контейнер, а не gateway, поэтому он не может
 *     дотянуться до картинки по публичному адресу. Этот handler читает
 *     INTERNAL_API_URL во время запроса (runtime) и ходит на `api-gateway:8080`.
 *
 * Почему route handler, а не rewrites() в next.config: destination у rewrites
 * запекается в routes-manifest на этапе сборки, когда INTERNAL_API_URL ещё не
 * задан (это runtime-переменная окружения), и подставлялся бы localhost:8080.
 */
export async function GET(
  req: NextRequest,
  { params }: { params: Promise<{ path: string[] }> },
) {
  const { path } = await params;
  const segments = (path ?? []).map(encodeURIComponent).join("/");
  if (!segments) {
    return new NextResponse("Not Found", { status: 404 });
  }

  const apiUrl =
    process.env.INTERNAL_API_URL ||
    process.env.NEXT_PUBLIC_API_URL ||
    "http://localhost:8080";

  let upstream: Response;
  try {
    upstream = await fetch(`${apiUrl}/api/v1/uploads/${segments}`, {
      // Кэшируем на стороне Next по заголовкам апстрима.
      headers: { Accept: req.headers.get("accept") ?? "*/*" },
    });
  } catch {
    return new NextResponse("Bad Gateway", { status: 502 });
  }

  if (!upstream.ok || !upstream.body) {
    return new NextResponse("Not Found", { status: upstream.status || 404 });
  }

  // Пробрасываем безопасные заголовки контента; стримим тело без буферизации.
  const headers = new Headers();
  for (const h of [
    "content-type",
    "content-length",
    "cache-control",
    "last-modified",
    "etag",
  ]) {
    const v = upstream.headers.get(h);
    if (v) headers.set(h, v);
  }
  if (!headers.has("cache-control")) {
    headers.set("cache-control", "public, max-age=3600");
  }

  return new NextResponse(upstream.body, {
    status: upstream.status,
    headers,
  });
}
