package provider

import (
	"fmt"

	brandrepo "github.com/wearwhere/wearwhere_be/internal/brand/repo"
)

type Config struct {
	Provider string // "flat" | "goship"
}

func NewFromConfig(cfg Config, brandRepo brandrepo.BrandRepo, gp *GoshipDeps) (ShippingProvider, error) {
	switch cfg.Provider {
	case "", "flat":
		return NewFlatRateProvider(brandRepo), nil
	case "goship":
		if gp == nil {
			return nil, fmt.Errorf("shipping: goship provider requires GoshipDeps")
		}
		return NewGoshipProvider(gp.Client, gp.PickupRepo, gp.Defaults), nil
	default:
		return nil, fmt.Errorf("unknown shipping provider: %q", cfg.Provider)
	}
}
