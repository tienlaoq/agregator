package domain

// ListPublicMastersParams — фильтры публичного каталога мастеров.
type ListPublicMastersParams struct {
	Query           string
	Cities          []string
	WorkFormat      string
	PriceMinKopecks int64
	PriceMaxKopecks int64
	Limit           int32
	Offset          int32
}
