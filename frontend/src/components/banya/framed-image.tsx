import Image from "next/image";
import { cn } from "@/lib/utils";

/**
 * Avito-style показ фото: картинка любого размера видна ЦЕЛИКОМ, без обрезки.
 *
 * Приём: на фон кладётся то же фото, растянутое (object-cover) и сильно
 * размытое + чуть затемнённое — оно заполняет всю рамку фиксированной
 * пропорции. Поверх — то же фото целиком (object-contain) по центру.
 * Пустых «полей» по бокам/сверху не остаётся, и при этом ничего не режется.
 *
 * Оба компонента позиционируются абсолютно и рассчитаны на размещение внутри
 * родителя с `position: relative`, `overflow-hidden` и заданной пропорцией
 * (например, `aspect-[4/3]` / `aspect-[16/9]`).
 */

// scale-110, чтобы размытые края не оголяли прозрачную рамку.
const BG_CLASS = "object-cover scale-110 blur-2xl brightness-95";

/** Вариант на next/image (оптимизация, sizes) — для каталожных карточек и галерей. */
export function FramedImage({
  src,
  alt,
  sizes,
  priority,
  unoptimized,
  className,
}: {
  src: string;
  alt: string;
  sizes?: string;
  priority?: boolean;
  unoptimized?: boolean;
  /** Доп. классы для переднего (несрезанного) изображения, напр. hover-зум. */
  className?: string;
}) {
  return (
    <>
      <Image
        src={src}
        alt=""
        aria-hidden
        fill
        sizes={sizes}
        priority={priority}
        unoptimized={unoptimized}
        className={BG_CLASS}
      />
      <Image
        src={src}
        alt={alt}
        fill
        sizes={sizes}
        priority={priority}
        unoptimized={unoptimized}
        className={cn("object-contain", className)}
      />
    </>
  );
}

/** Вариант на обычном <img> — для мест, где next/image не используется. */
export function FramedImg({
  src,
  alt,
  className,
}: {
  src: string;
  alt: string;
  className?: string;
}) {
  return (
    <>
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img
        src={src}
        alt=""
        aria-hidden
        className={cn("absolute inset-0 h-full w-full", BG_CLASS)}
      />
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img
        src={src}
        alt={alt}
        className={cn("absolute inset-0 h-full w-full object-contain", className)}
      />
    </>
  );
}
