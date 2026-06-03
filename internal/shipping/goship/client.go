package goship

import (
	"context"
	"errors"
)

var (
	ErrRates    = errors.New("goship: failed to fetch rates")
	ErrLocation = errors.New("goship: failed to fetch location list")
)

// Location is a city, district, or ward as returned by Goship.
// Goship codes are numeric strings, e.g. "100000".
type Location struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Address is one endpoint of a shipment (sender or receiver).
type Address struct {
	DistrictCode string
	CityCode     string
}

// Parcel describes the package being shipped. We send actual weight + dims;
// Goship applies volumetric weight (divisor ~6000) server-side.
type Parcel struct {
	WeightG   int
	LengthCM  int
	WidthCM   int
	HeightCM  int
	CODVND    int64 // amount the carrier collects on delivery (0 for prepaid/PayOS)
	AmountVND int64 // declared goods value
}

type RateReq struct {
	From   Address
	To     Address
	Parcel Parcel
}

// Rate is one carrier option returned by Goship.
type Rate struct {
	ID          string // Goship rate id (short-lived; not persisted in Spec A)
	Carrier     string // carrier code (e.g. "ghnv3"); falls back to CarrierName if Goship omits a code
	CarrierName string
	Service     string
	FeeVND      int64
	ETA         string // human-readable expected delivery ("expected")
}

type Client interface {
	Cities(ctx context.Context) ([]Location, error)
	Districts(ctx context.Context, cityCode string) ([]Location, error)
	Wards(ctx context.Context, districtCode string) ([]Location, error)
	Rates(ctx context.Context, r RateReq) ([]Rate, error)
}
