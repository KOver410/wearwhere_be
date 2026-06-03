package domain

type ShippingOption struct {
	Carrier     string // "" / "flat" for flat-rate; carrier code for goship
	CarrierName string
	Service     string
	AmountVND   int64
	ETA         string
}
