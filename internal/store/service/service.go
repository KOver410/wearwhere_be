// Package service orchestrates store discovery: DB candidates + Goong enrichment.
package service

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/maps/goong"
	"github.com/wearwhere/wearwhere_be/internal/store/domain"
	"github.com/wearwhere/wearwhere_be/internal/store/repo"
)

const (
	defaultRadiusKm = 5.0
	widenRadiusKm   = 10.0
	maxNearby       = 25
)

type Service struct {
	repo  repo.Repo
	goong goong.Client
	now   func() time.Time
}

func New(r repo.Repo, g goong.Client) *Service {
	return &Service{repo: r, goong: g, now: time.Now}
}

func (s *Service) Nearby(ctx context.Context, lat, lng, radiusKm float64) ([]domain.StoreSummary, error) {
	if radiusKm <= 0 {
		radiusKm = defaultRadiusKm
	}
	stores, err := s.repo.Nearby(ctx, lat, lng, radiusKm, maxNearby)
	if err != nil {
		return nil, err
	}
	if len(stores) == 0 && radiusKm < widenRadiusKm {
		stores, err = s.repo.Nearby(ctx, lat, lng, widenRadiusKm, maxNearby)
		if err != nil {
			return nil, err
		}
	}
	s.enrichDistances(ctx, lat, lng, stores)
	sort.SliceStable(stores, func(i, j int) bool {
		return derefM(stores[i].DistanceM) < derefM(stores[j].DistanceM)
	})
	out := make([]domain.StoreSummary, 0, len(stores))
	for _, st := range stores {
		out = append(out, domain.ToStoreSummary(st, domain.ComputeOpenStatus(st.Hours, s.now())))
	}
	return out, nil
}

// enrichDistances sets DistanceM/DurationS from Goong; on failure falls back to
// haversine meters with DistanceApprox=true (graceful degradation per spec).
func (s *Service) enrichDistances(ctx context.Context, lat, lng float64, stores []*domain.Store) {
	if len(stores) == 0 {
		return
	}
	dests := make([]goong.LatLng, len(stores))
	for i, st := range stores {
		dests[i] = goong.LatLng{Lat: st.Latitude, Lng: st.Longitude}
	}
	res, err := s.goong.DistanceMatrix(ctx, goong.LatLng{Lat: lat, Lng: lng}, dests)
	if err != nil || len(res) != len(stores) {
		for _, st := range stores {
			m := int64(domain.HaversineKm(lat, lng, st.Latitude, st.Longitude) * 1000)
			st.DistanceM = &m
			st.DistanceApprox = true
		}
		return
	}
	for i, st := range stores {
		dm, du := res[i].DistanceM, res[i].DurationS
		st.DistanceM, st.DurationS = &dm, &du
	}
}

func (s *Service) SearchByArea(ctx context.Context, f repo.AreaFilter, origin *goong.LatLng) ([]domain.StoreSummary, error) {
	stores, err := s.repo.SearchByArea(ctx, f)
	if err != nil {
		return nil, err
	}
	if origin != nil {
		s.enrichDistances(ctx, origin.Lat, origin.Lng, stores)
	}
	out := make([]domain.StoreSummary, 0, len(stores))
	for _, st := range stores {
		out = append(out, domain.ToStoreSummary(st, domain.ComputeOpenStatus(st.Hours, s.now())))
	}
	return out, nil
}

func (s *Service) Detail(ctx context.Context, id uuid.UUID) (domain.StoreDetail, error) {
	st, err := s.repo.Detail(ctx, id)
	if err != nil {
		if err == repo.ErrNotFound {
			return domain.StoreDetail{}, domain.ErrStoreNotFound()
		}
		return domain.StoreDetail{}, err
	}
	return domain.ToStoreDetail(st, domain.ComputeOpenStatus(st.Hours, s.now())), nil
}

func (s *Service) Directions(ctx context.Context, id uuid.UUID, from goong.LatLng) (domain.DirectionsResponse, error) {
	st, err := s.repo.Detail(ctx, id)
	if err != nil {
		if err == repo.ErrNotFound {
			return domain.DirectionsResponse{}, domain.ErrStoreNotFound()
		}
		return domain.DirectionsResponse{}, err
	}
	route, err := s.goong.Directions(ctx, from, goong.LatLng{Lat: st.Latitude, Lng: st.Longitude})
	if err != nil {
		return domain.DirectionsResponse{}, domain.ErrDirectionsUnavailable()
	}
	return domain.DirectionsResponse{DistanceM: route.DistanceM, DurationS: route.DurationS, Polyline: route.Polyline}, nil
}

func derefM(p *int64) int64 {
	if p == nil {
		return 1 << 62 // sort missing distances last
	}
	return *p
}
