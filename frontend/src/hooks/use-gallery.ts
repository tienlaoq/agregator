"use client"

import { useCallback, useEffect, useState } from "react"
// React 19 pattern for "reset state when a prop changes": compare a tracked
// value during render instead of writing state from inside useEffect.
// See https://react.dev/reference/react/useState#storing-information-from-previous-renders

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

  // Сбрасываем при смене объекта.
  // Render-time reset (вместо useEffect): React 19 заново отрендерит этот
  // компонент со сброшенным состоянием без cascading re-render.
  const [prevResetKey, setPrevResetKey] = useState(resetKey)
  if (resetKey !== prevResetKey) {
    setPrevResetKey(resetKey)
    setIndex(0)
    setLightboxOpen(false)
  }

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

  // Переключить текущий слайд без открытия лайтбокса (клик по миниатюре).
  const goTo = useCallback(
    (idx: number) => {
      if (count === 0) return
      setIndex(((idx % count) + count) % count)
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

  // Блокируем прокрутку фона, пока открыт лайтбокс.
  useEffect(() => {
    if (!lightboxOpen) return
    const prevOverflow = document.body.style.overflow
    document.body.style.overflow = "hidden"
    return () => {
      document.body.style.overflow = prevOverflow
    }
  }, [lightboxOpen])

  return {
    index: safeIndex,
    current,
    lightboxOpen,
    openAt,
    goTo,
    closeLightbox,
    prev,
    next,
    count,
  }
}
