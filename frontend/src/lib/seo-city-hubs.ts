/**
 * Посадочные URL вида /venues/city/{slug} и /venues/city/{slug}/{kind}.
 * Слаг — латиница; name — как в фильтре каталога (должен совпадать с city в БД / поиске).
 */
export type SeoCityHub = {
  slug: string;
  name: string;
  /** Уникальный абзац для SEO (не копипаст между городами). */
  intro: string;
};

export const SEO_CITY_HUBS: SeoCityHub[] = [
  {
    slug: "moskva",
    name: "Москва",
    intro:
      "В Москве высокий спрос на частные и клубные бани с парилкой разного типа — от классической русской до финской сухой сауны. Подборка на БаняГид помогает сравнить цены, вместимость залов и реальные отзывы гостей без обхода десятков сайтов.",
  },
  {
    slug: "sankt-peterburg",
    name: "Санкт-Петербург",
    intro:
      "Петербургские бани и сауны часто совмещают парение с отдыхом у воды или в историческом центре. Здесь собраны площадки с онлайн-бронированием: удобно заранее забронировать зал на компанию или семейный визит.",
  },
  {
    slug: "kazan",
    name: "Казань",
    intro:
      "В Казани популярны и общественные банные традиции, и современные SPA-форматы. Каталог позволяет отфильтровать заведения по району и типу парения, не теряясь в соцсетях и мессенджерах владельцев.",
  },
  {
    slug: "novosibirsk",
    name: "Новосибирск",
    intro:
      "Для Новосибирска важны тёплые зоны отдыха в холодный сезон: сауны с бассейном и зоной отдыха особенно востребованы. Сравните варианты по рейтингу и отзывам перед бронированием.",
  },
  {
    slug: "ekaterinburg",
    name: "Екатеринбург",
    intro:
      "В Екатеринбурге много форматов — от компактных саун в жилых кварталах до просторных банных комплексов. Используйте фильтры по цене и типу, чтобы быстрее найти место под ваш сценарий отдыха.",
  },
  {
    slug: "nizhniy-novgorod",
    name: "Нижний Новгород",
    intro:
      "Нижегородские гости ценят прозрачные условия бронирования и честные фото залов. На этой странице — только заведения, отфильтрованные по городу; детали и слоты уточняются на карточке.",
  },
  {
    slug: "krasnodar",
    name: "Краснодар",
    intro:
      "В Краснодаре и пригороде много частных бань на компанию и семейные сауны с бассейном. Подборка обновляется по мере появления новых площадок в каталоге БаняГид.",
  },
  {
    slug: "samara",
    name: "Самара",
    intro:
      "Самарский рынок бань сочетает доступные почасовые тарифы и премиальные комплексы. Отфильтруйте по рейтингу и типу парения, чтобы не тратить время на заведения вне города.",
  },
];

export const SEO_VENUE_KIND_SLUGS = ["banya", "sauna", "hammam"] as const;
export type SeoVenueKindSlug = (typeof SEO_VENUE_KIND_SLUGS)[number];

export const SEO_VENUE_KIND_LABELS: Record<SeoVenueKindSlug, string> = {
  banya: "Бани",
  sauna: "Сауны",
  hammam: "Хаммамы",
};

export const SEO_VENUE_KIND_INTRO: Record<SeoVenueKindSlug, string> = {
  banya:
    "Ниже — бани с русской или классической парилкой: парение, веники, отдых между заходами. Уточняйте вместимость залов и услуги на карточке.",
  sauna:
    "Подборка саун с сухим паром и регулируемой влажностью: удобно тем, кто предпочитает мягкое тепло и короткие заходы.",
  hammam:
    "Хаммамы и турецкие бани с мягким паром и высокой влажностью — хороший вариант для первого опыта парения и SPA-релакса.",
};

const cityBySlug = new Map(SEO_CITY_HUBS.map((c) => [c.slug, c]));

export function getSeoCityHub(slug: string): SeoCityHub | undefined {
  return cityBySlug.get(slug);
}

export function isSeoVenueKindSlug(s: string): s is SeoVenueKindSlug {
  return (SEO_VENUE_KIND_SLUGS as readonly string[]).includes(s);
}

export function allCitySlugsForStaticParams(): { citySlug: string }[] {
  return SEO_CITY_HUBS.map((c) => ({ citySlug: c.slug }));
}

export function allCityKindStaticParams(): { citySlug: string; kind: SeoVenueKindSlug }[] {
  const out: { citySlug: string; kind: SeoVenueKindSlug }[] = [];
  for (const c of SEO_CITY_HUBS) {
    for (const kind of SEO_VENUE_KIND_SLUGS) {
      out.push({ citySlug: c.slug, kind });
    }
  }
  return out;
}
