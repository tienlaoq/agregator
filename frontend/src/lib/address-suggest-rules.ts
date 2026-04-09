/** Strip leading street type; remainder is the typed street name. */
export function extractStreetCore(segment: string): string {
  let s = segment.trim()
  s = s.replace(
    /^(?:ул\.?|улица|пр-?т\.?|проспект|пер\.?|переулок|бул\.?|бульвар|наб\.?|набережная|ш\.?|шоссе|пр-д\.?|проезд|туп\.?|тупик|ал\.?|аллея|лин\.?|линия|мкр\.?|микрорайон)\s+/i,
    "",
  )
  return s.trim()
}

/** True when user has typed enough of the first line (min half, at least 4 chars in core). */
export function isStreetSuggestReady(fullLine: string): boolean {
  const firstSegment = fullLine.split(",")[0]?.trim() ?? fullLine.trim()
  if (firstSegment.length < 2) return false
  const core = extractStreetCore(firstSegment)
  const needLen = Math.max(4, Math.ceil(firstSegment.length / 2))
  return core.length >= needLen
}
