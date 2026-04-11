/** Роли с доступом к кабинету заведения (совпадает с RequireRole в api-gateway). */
export const PARTNER_CABINET_ROLES = ["venue_owner", "master"] as const

export type PartnerCabinetRole = (typeof PARTNER_CABINET_ROLES)[number]

export function isPartnerCabinetRole(role: string | undefined | null): boolean {
  if (!role) return false
  return (PARTNER_CABINET_ROLES as readonly string[]).includes(role)
}
