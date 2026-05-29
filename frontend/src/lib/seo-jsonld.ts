/** Safe serialization for embedding JSON-LD into <script> tag. */
export function safeJsonLdStringify(value: unknown): string {
  // Prevent breaking out of script context (e.g. </script> inside user-generated content).
  return JSON.stringify(value).replace(/</g, "\\u003c")
}

