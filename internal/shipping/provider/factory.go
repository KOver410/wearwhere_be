package provider

import (
	"fmt"

	brandrepo "github.com/wearwhere/wearwhere_be/internal/brand/repo"
)

type Config struct {
	Provider string // "flat" (Sprint 3); future: "ghn", "ghtk", "viettelpost"
}

func NewFromConfig(cfg Config, brandRepo brandrepo.BrandRepo) (ShippingProvider, error) {
	switch cfg.Provider {
	case "", "flat":
		return NewFlatRateProvider(brandRepo), nil
	default:
		return nil, fmt.Errorf("unknown shipping provider: %q", cfg.Provider)
	}
}
