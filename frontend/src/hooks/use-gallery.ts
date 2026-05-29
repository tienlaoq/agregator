"use client"

import { useCallback, useEffect, useState } from "react"

export interface GallerySlide {
  id: string
  src: string
}

/**
 * Управляет состоянием галереи/карусели: текущий индекс, лайтбокс, навигация.
 * Используется в venue-public-page-client и master-public-page-client.
 *
 * @param slides — массив слайдов
 * @param resetKey — при изменении (slug, id и т.п.) сбрасывает индекс в 0
 */
export function useGallery(slides: GallerySlide[], resetKey?: string | null) {
  const [index, setIndex] = useState(0)
  const [lightboxOpen, setLightboxOpen] = useState(false)

  // Сбрасываем при смене объекта
  useEffect(() => {
    setIndex(0)
    setLightboxOpen(false)
  }, [resetKey])

  const count = slides.length
  const safeIndex = count > 0 ? Math.min(index, count - 1) : 0
  const current = count > 0 ? slides[safeIndex] : null

  const prev = useCallback(() => {
    if (count === 0) return
    setIndex((i) => (i - 1 + count) % count)
  }, [count])

  const next = useCallback(() => {
    if (count === 0) return
    setIndex((i) => (i + 1) % count)
  }, [count])

  const openAt = useCallback(
    (idx: number) => {
      if (count === 0) return
      const safe = ((idx % count) + count) % count
      setIndex(safe)
      setLightboxOpen(true)
    },
    [count],
  )

  const closeLightbox = useCallback(() => setLightboxOpen(false), [])

  // Клавиатурная навигация в лайтбоксе
  useEffect(() => {
    if (!lightboxOpen || count === 0) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "ArrowLeft") { e.preventDefault(); prev() }
      if (e.key === "ArrowRight") { e.preventDefault(); next() }
      if (e.key === "Escape") { e.preventDefault(); closeLightbox() }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [lightboxOpen, count, prev, next, closeLightbox])

  return {
    index: safeIndex,
    current,
    lightboxOpen,
    openAt,
    closeLightbox,
    prev,
    next,
    count,
  }
}
