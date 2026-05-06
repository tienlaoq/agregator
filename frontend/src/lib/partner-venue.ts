import {
  DEFAULT_VENUE_SOCIAL_LINKS,
  type CreateVenueRequest,
  type UpdateVenueRequest,
} from "./types";

/** Поля шага «Расскажите о заведении» в регистрации партнёра и в кабинете «Добавить заведение». */
export type PartnerVenueFormValues = {
  venueName: string;
  venueType: string;
  venueCity: string;
  venueAddress: string;
  venueDescription: string;
  legalEntityName: string;
  inn: string;
  ogrn: string;
  publicListingUrl: string;
  verificationNote: string;
};

export function emptyPartnerVenueForm(): PartnerVenueFormValues {
  return {
    venueName: "",
    venueType: "",
    venueCity: "",
    venueAddress: "",
    venueDescription: "",
    legalEntityName: "",
    inn: "",
    ogrn: "",
    publicListingUrl: "",
    verificationNote: "",
  };
}

export function isOwnerVerificationComplete(
  legal: string,
  innRaw: string,
  ogrnRaw: string,
  listingUrl: string,
): boolean {
  const legalTrim = legal.trim();
  if (legalTrim.length < 3) return false;
  const innD = innRaw.replace(/\D/g, "");
  if (innD.length !== 10 && innD.length !== 12) return false;
  const ogrnD = ogrnRaw.replace(/\D/g, "");
  if (innD.length === 10 && ogrnD.length !== 13) return false;
  if (innD.length === 12 && ogrnD.length !== 15) return false;
  const u = listingUrl.trim();
  try {
    const parsed = new URL(u);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") return false;
    const h = parsed.hostname.toLowerCase();
    if (h === "localhost" || h.startsWith("127.")) return false;
  } catch {
    return false;
  }
  return true;
}

/** Та же логика кнопки «Далее» на шаге заведения у партнёра. */
export function partnerVenueStepCanProceed(v: PartnerVenueFormValues): boolean {
  return (
    !!v.venueName.trim() &&
    !!v.venueType &&
    !!v.venueCity.trim() &&
    isOwnerVerificationComplete(v.legalEntityName, v.inn, v.ogrn, v.publicListingUrl)
  );
}

/** Тело POST /venues как при регистрации владельца-сети (черновик, без залов в теле). */
export function buildPartnerStyleCreateVenueRequest(
  v: PartnerVenueFormValues,
  phoneRaw: string,
): CreateVenueRequest {
  return {
    name: v.venueName.trim(),
    type: v.venueType as CreateVenueRequest["type"],
    city: v.venueCity.trim(),
    address: v.venueAddress.trim(),
    description: v.venueDescription.trim(),
    phone: phoneRaw,
    price_from: 0,
    capacity: 0,
    amenities: [],
    halls: [],
    services: [],
    legal_entity_name: v.legalEntityName.trim(),
    inn: v.inn.replace(/\D/g, ""),
    ogrn: v.ogrn.replace(/\D/g, ""),
    public_listing_url: v.publicListingUrl.trim(),
    verification_note: v.verificationNote.trim() || undefined,
    social_links: { ...DEFAULT_VENUE_SOCIAL_LINKS },
    start_as_draft: true,
  };
}

export function venueTypeBadgeLabel(type: string): string {
  if (type === "banya") return "Баня";
  if (type === "sauna") return "Сауна";
  if (type === "hammam") return "Хаммам";
  return type || "—";
}

/** Срез формы редактирования карточки в модель общей карточки «о заведении». */
export function partnerVenueValuesFromUpdateForm(
  f: UpdateVenueRequest,
  venueType: string,
): PartnerVenueFormValues {
  return {
    venueName: f.name,
    venueType,
    venueCity: f.city,
    venueAddress: f.address,
    venueDescription: f.description,
    legalEntityName: f.legal_entity_name,
    inn: f.inn,
    ogrn: f.ogrn,
    publicListingUrl: f.public_listing_url,
    verificationNote: f.verification_note ?? "",
  };
}

/** Применить правки из общей карточки к PATCH-форме (тип заведения в теле PATCH не передаётся). */
export function applyPartnerVenuePatchToUpdateForm(
  f: UpdateVenueRequest,
  patch: Partial<PartnerVenueFormValues>,
): UpdateVenueRequest {
  const next = { ...f };
  if (patch.venueName !== undefined) next.name = patch.venueName;
  if (patch.venueCity !== undefined) next.city = patch.venueCity;
  if (patch.venueAddress !== undefined) next.address = patch.venueAddress;
  if (patch.venueDescription !== undefined) next.description = patch.venueDescription;
  if (patch.legalEntityName !== undefined) next.legal_entity_name = patch.legalEntityName;
  if (patch.inn !== undefined) next.inn = patch.inn;
  if (patch.ogrn !== undefined) next.ogrn = patch.ogrn;
  if (patch.publicListingUrl !== undefined) next.public_listing_url = patch.publicListingUrl;
  if (patch.verificationNote !== undefined) {
    const t = patch.verificationNote.trim();
    next.verification_note = t ? t : undefined;
  }
  return next;
}
