package domain

import "time"

type FeeQuote struct {
	AmountVND   int64
	Currency    string         // "VND"
	ProviderRef string         // vendor quote id (Sprint 4+ for re-price)
	ETA         *time.Duration // optional delivery time
}
