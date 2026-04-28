const EARTH_RADIUS_KM = 6371;

/** Допуск при сравнении «зона исключения внутри зоны выезда» (карта / округление км). */
export const TRAVEL_ZONE_CONTAINMENT_EPS_KM = 0.05;

/** Расстояние по дуге большого круга между двумя точками WGS84, км. */
export function haversineKm(lat1: number, lon1: number, lat2: number, lon2: number): number {
  const rad = Math.PI / 180;
  const φ1 = lat1 * rad;
  const φ2 = lat2 * rad;
  const Δφ = (lat2 - lat1) * rad;
  const Δλ = (lon2 - lon1) * rad;
  const sΔφ = Math.sin(Δφ / 2);
  const sΔλ = Math.sin(Δλ / 2);
  const a = sΔφ * sΔφ + Math.cos(φ1) * Math.cos(φ2) * sΔλ * sΔλ;
  const c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
  return EARTH_RADIUS_KM * c;
}

/** Круг исключения целиком внутри зоны выезда радиуса `travelRadiusKm` от метки. */
export function excludeZoneContainedInTravelRadius(
  pinLat: number,
  pinLon: number,
  travelRadiusKm: number,
  zoneLat: number,
  zoneLon: number,
  zoneRadiusKm: number,
  epsKm = TRAVEL_ZONE_CONTAINMENT_EPS_KM,
): boolean {
  if (!Number.isFinite(travelRadiusKm) || travelRadiusKm <= 0) return false;
  const d = haversineKm(pinLat, pinLon, zoneLat, zoneLon);
  return d + zoneRadiusKm <= travelRadiusKm + epsKm;
}
