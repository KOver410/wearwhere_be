package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/maps/goong"
	"github.com/wearwhere/wearwhere_be/internal/store/domain"
	"github.com/wearwhere/wearwhere_be/internal/store/repo"
)

type fakeRepo struct {
	nearby     []*domain.Store
	detail     *domain.Store
	nearbyFunc func(radiusKm float64) []*domain.Store // optional radius-aware override
}

func (f *fakeRepo) Nearby(_ context.Context, _, _, radiusKm float64, _ int) ([]*domain.Store, error) {
	if f.nearbyFunc != nil {
		return f.nearbyFunc(radiusKm), nil
	}
	return f.nearby, nil
}
func (f *fakeRepo) SearchByArea(_ context.Context, _ repo.AreaFilter) ([]*domain.Store, error) {
	return f.nearby, nil
}
func (f *fakeRepo) Detail(_ context.Context, _ uuid.UUID) (*domain.Store, error) {
	if f.detail == nil {
		return nil, repo.ErrNotFound
	}
	return f.detail, nil
}

// failGoong errors on DistanceMatrix to exercise the degrade path.
type failGoong struct{ goong.Client }

func (failGoong) DistanceMatrix(context.Context, goong.LatLng, []goong.LatLng) ([]goong.DistanceResult, error) {
	return nil, errors.New("boom")
}

func TestNearby_EnrichesWithGoongDistance(t *testing.T) {
	stores := []*domain.Store{
		{AddressID: uuid.New(), Latitude: 10.78, Longitude: 106.70},
		{AddressID: uuid.New(), Latitude: 10.80, Longitude: 106.71},
	}
	svc := New(&fakeRepo{nearby: stores}, goong.NewMockClient())
	got, err := svc.Nearby(context.Background(), 10.7769, 106.7009, 0)
	if err != nil {
		t.Fatalf("Nearby: %v", err)
	}
	if got[0].DistanceM == nil {
		t.Fatal("expected DistanceM populated from Goong")
	}
	if *got[0].DistanceM > *got[1].DistanceM {
		t.Error("expected ascending distance order")
	}
}

func TestNearby_DegradesWhenGoongFails(t *testing.T) {
	stores := []*domain.Store{{AddressID: uuid.New(), Latitude: 10.78, Longitude: 106.70}}
	svc := New(&fakeRepo{nearby: stores}, failGoong{goong.NewMockClient()})
	got, err := svc.Nearby(context.Background(), 10.7769, 106.7009, 0)
	if err != nil {
		t.Fatalf("Nearby should degrade, not fail: %v", err)
	}
	if !got[0].DistanceApprox || got[0].DistanceM == nil {
		t.Errorf("expected haversine fallback with DistanceApprox=true, got %+v", got[0])
	}
}

func TestNearby_WidensRadiusWhenEmpty(t *testing.T) {
	store := &domain.Store{AddressID: uuid.New(), Latitude: 10.85, Longitude: 106.75}
	calls := []float64{}
	repo := &fakeRepo{nearbyFunc: func(radiusKm float64) []*domain.Store {
		calls = append(calls, radiusKm)
		if radiusKm >= 10 {
			return []*domain.Store{store}
		}
		return nil // empty at 5km
	}}
	svc := New(repo, goong.NewMockClient())
	got, err := svc.Nearby(context.Background(), 10.7769, 106.7009, 0) // radius 0 → default 5
	if err != nil {
		t.Fatalf("Nearby: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 store after widening, got %d", len(got))
	}
	if len(calls) != 2 || calls[0] != 5 || calls[1] != 10 {
		t.Errorf("expected calls at 5km then 10km, got %v", calls)
	}
}
