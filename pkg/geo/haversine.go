package geo

import "math"

const earthRadiusKm = 6371.0

// HaversineKm returns great-circle distance between two WGS84 points in kilometers.
func HaversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const rad = math.Pi / 180
	φ1 := lat1 * rad
	φ2 := lat2 * rad
	Δφ := (lat2 - lat1) * rad
	Δλ := (lon2 - lon1) * rad
	sΔφ := math.Sin(Δφ / 2)
	sΔλ := math.Sin(Δλ / 2)
	a := sΔφ*sΔφ + math.Cos(φ1)*math.Cos(φ2)*sΔλ*sΔλ
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusKm * c
}
