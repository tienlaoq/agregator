/**
 * Validates a post-login redirect target so it cannot be abused for an open
 * redirect. Only same-site absolute paths are honoured: the value must start
 * with a single "/" and must not start with "//" or "/\", both of which the
 * browser resolves to an external origin.
 *
 * The argument is expected to already be percent-decoded (e.g. read via
 * URLSearchParams or next/navigation's useSearchParams). Returns the path, or
 * null when it is missing or unsafe — callers fall back to their default.
 */
export function safeNextPath(raw: string | null | undefined): string | null {
  if (!raw || raw[0] !== "/") return null;
  if (raw.length > 1 && (raw[1] === "/" || raw[1] === "\\")) return null;
  return raw;
}
