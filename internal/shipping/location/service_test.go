package location

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
)

type countingClient struct{ calls atomic.Int64 }

func (c *countingClient) Cities(context.Context) ([]goship.Location, error) {
	c.calls.Add(1)
	return []goship.Location{{Code: "100000", Name: "HCM"}}, nil
}
func (c *countingClient) Districts(context.Context, string) ([]goship.Location, error) { return nil, nil }
func (c *countingClient) Wards(context.Context, string) ([]goship.Location, error)     { return nil, nil }
func (c *countingClient) Rates(context.Context, goship.RateReq) ([]goship.Rate, error) { return nil, nil }

func TestService_Cities_CachedWithinTTL(t *testing.T) {
	cc := &countingClient{}
	s := NewService(cc, time.Hour)
	for i := 0; i < 3; i++ {
		if _, err := s.Cities(context.Background()); err != nil {
			t.Fatalf("Cities: %v", err)
		}
	}
	if got := cc.calls.Load(); got != 1 {
		t.Errorf("client called %d times, want 1 (cached)", got)
	}
}
