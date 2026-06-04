package location

import (
	"context"
	"sync"
	"time"

	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
)

type cacheEntry struct {
	data    []goship.Location
	expires time.Time
}

type Service struct {
	client goship.Client
	ttl    time.Duration
	mu     sync.Mutex
	cache  map[string]cacheEntry
}

func NewService(c goship.Client, ttl time.Duration) *Service {
	return &Service{client: c, ttl: ttl, cache: map[string]cacheEntry{}}
}

func (s *Service) get(ctx context.Context, key string, load func(context.Context) ([]goship.Location, error)) ([]goship.Location, error) {
	s.mu.Lock()
	if e, ok := s.cache[key]; ok && time.Now().Before(e.expires) {
		s.mu.Unlock()
		return e.data, nil
	}
	s.mu.Unlock()

	data, err := load(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cache[key] = cacheEntry{data: data, expires: time.Now().Add(s.ttl)}
	s.mu.Unlock()
	return data, nil
}

func (s *Service) Cities(ctx context.Context) ([]goship.Location, error) {
	return s.get(ctx, "cities", s.client.Cities)
}

func (s *Service) Districts(ctx context.Context, cityCode string) ([]goship.Location, error) {
	return s.get(ctx, "d:"+cityCode, func(c context.Context) ([]goship.Location, error) {
		return s.client.Districts(c, cityCode)
	})
}

func (s *Service) Wards(ctx context.Context, districtCode string) ([]goship.Location, error) {
	return s.get(ctx, "w:"+districtCode, func(c context.Context) ([]goship.Location, error) {
		return s.client.Wards(c, districtCode)
	})
}
