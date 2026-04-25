package usecase

// PlatformFeeKopecks returns the platform cut in kopecks: gross * feeBPS / 10000 (trunc toward zero).
func PlatformFeeKopecks(grossKopecks, feeBPS int64) int64 {
	if grossKopecks <= 0 || feeBPS <= 0 {
		return 0
	}
	return grossKopecks * feeBPS / 10000
}

// CounterpartyNetKopecks is the seller portion after the platform fee (same units as gross).
func CounterpartyNetKopecks(grossKopecks, platformFeeKopecks int64) int64 {
	n := grossKopecks - platformFeeKopecks
	if n < 0 {
		return 0
	}
	return n
}
