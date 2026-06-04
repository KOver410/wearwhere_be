package domain

type ShippingOption struct {
	Provider    string // which provider produced this option: "flat" | "goship"
	Carrier     string // "" / "flat" for flat-rate; carrier code for goship
	CarrierName string
	Service     string
	AmountVND   int64
	ETA         string
}
