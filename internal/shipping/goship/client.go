package goship

import (
	"context"
	"errors"
)

var (
	ErrRates            = errors.New("goship: failed to fetch rates")
	ErrLocation         = errors.New("goship: failed to fetch location list")
	ErrCreateShipment   = errors.New("goship: failed to create shipment")
	ErrSignatureInvalid = errors.New("goship: invalid webhook signature")
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

// ShipmentAddress is a sender/recipient for shipment creation.
type ShipmentAddress struct {
	Name         string
	Phone        string
	Street       string
	WardCode     string
	DistrictCode string
	CityCode     string
}

type ShipmentReq struct {
	RateID   string
	From     ShipmentAddress
	To       ShipmentAddress
	Parcel   Parcel
	OrderRef string
}

type ShipmentResp struct {
	TrackingCode string
	GoshipCode   string
	LabelURL     string
	FeeVND       int64
}

type WebhookPayload struct {
	GCode        string `json:"gcode"`
	Code         string `json:"code"`
	OrderID      string `json:"order_id"`
	Status       string `json:"status"`
	StatusText   string `json:"status_text"`
	Message      string `json:"message"`
	TrackingURL  string `json:"tracking_url"`
	IsReturn     int    `json:"is_return"`
	IsLost       int    `json:"is_lost"`
	CarrierShort string `json:"carrier_short_name"`
	UpdateTime   int64  `json:"update_time"`
}

type Shipper interface {
	CreateShipment(ctx context.Context, r ShipmentReq) (*ShipmentResp, error)
	VerifyWebhookSignature(rawBody []byte, signature string) error
}

type Service interface {
	Client
	Shipper
}
