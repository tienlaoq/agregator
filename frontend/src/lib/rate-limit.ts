/**
 * In-memory IP-based sliding window rate limiter for Next.js API routes.
 *
 * Designed for Node.js runtime (not Edge). Each Limiter instance maintains its
 * own map, so create one per route at module scope (singleton pattern).
 *
 * Usage:
 *   const limiter = new RateLimiter({ maxRequests: 20, windowMs: 60_000 })
 *   const ip = req.headers.get("x-forwarded-for")?.split(",")[0].trim() ?? "unknown"
 *   if (!limiter.check(ip)) {
 *     return NextResponse.json({ error: "rate_limited" }, { status: 429 })
 *   }
 */

interface LimiterEntry {
  /** Timestamps of requests in the current window (ms). */
  timestamps: number[]
  /** setTimeout handle for cleanup. */
  timer: ReturnType<typeof setTimeout>
}

interface RateLimiterOptions {
  /** Maximum allowed requests per IP within `windowMs`. */
  maxRequests: number
  /** Window duration in milliseconds. */
  windowMs: number
}

export class RateLimiter {
  private readonly maxRequests: number
  private readonly windowMs: number
  private readonly map = new Map<string, LimiterEntry>()

  constructor({ maxRequests, windowMs }: RateLimiterOptions) {
    this.maxRequests = maxRequests
    this.windowMs = windowMs
  }

  /**
   * Returns `true` if the request is allowed, `false` if rate-limited.
   * Silently drops timestamps outside the current window on each call.
   */
  check(ip: string): boolean {
    const now = Date.now()
    const cutoff = now - this.windowMs

    let entry = this.map.get(ip)
    if (!entry) {
      entry = {
        timestamps: [],
        timer: setTimeout(() => this.map.delete(ip), this.windowMs * 2),
      }
      // Don't block the process from exiting because of this timer.
      entry.timer.unref?.()
      this.map.set(ip, entry)
    }

    // Slide the window — remove stale timestamps.
    entry.timestamps = entry.timestamps.filter((t) => t > cutoff)

    if (entry.timestamps.length >= this.maxRequests) {
      return false
    }

    entry.timestamps.push(now)

    // Reset cleanup timer so the entry lives at least `windowMs * 2` after last request.
    clearTimeout(entry.timer)
    entry.timer = setTimeout(() => this.map.delete(ip), this.windowMs * 2)
    entry.timer.unref?.()

    return true
  }

  /** Exposed for testing. */
  get size(): number {
    return this.map.size
  }
}

/**
 * Extract the real client IP from a Next.js `NextRequest`.
 * Prefers `x-forwarded-for` (set by Vercel / nginx reverse proxy),
 * falls back to a placeholder so the limiter always has a key.
 */
export function getClientIp(headers: Headers): string {
  // x-forwarded-for may contain a comma-separated list; first entry is the client.
  const forwarded = headers.get("x-forwarded-for")
  if (forwarded) {
    const first = forwarded.split(",")[0].trim()
    if (first) return first
  }
  // x-real-ip is set by nginx.
  const realIp = headers.get("x-real-ip")
  if (realIp) return realIp.trim()
  return "unknown"
}
