package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/brand/domain"
	"github.com/wearwhere/wearwhere_be/internal/brand/repo"
	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
	"github.com/wearwhere/wearwhere_be/internal/shipping/location"
)

type Service struct {
	brands    repo.BrandRepo
	addresses repo.AddressRepo
	loc       *location.Service
}

func New(b repo.BrandRepo, a repo.AddressRepo, loc *location.Service) *Service {
	return &Service{brands: b, addresses: a, loc: loc}
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*domain.Brand, error) {
	b, err := s.brands.FindByID(ctx, id)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, domain.ErrBrandNotFound
	}
	return b, err
}

func (s *Service) GetBySlug(ctx context.Context, slug string) (*domain.Brand, error) {
	b, err := s.brands.FindBySlug(ctx, slug)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, domain.ErrBrandNotFound
	}
	return b, err
}

func (s *Service) UpdateOwn(ctx context.Context, id uuid.UUID, req *domain.UpdateBrandRequest) error {
	err := s.brands.Update(ctx, id, req)
	switch {
	case errors.Is(err, repo.ErrNotFound):
		return domain.ErrBrandNotFound
	case errors.Is(err, repo.ErrSlugTaken):
		return domain.ErrSlugTaken
	}
	return err
}

func (s *Service) ListBrands(ctx context.Context, q, sort string, limit, offset int) ([]*domain.Brand, int, error) {
	return s.brands.List(ctx, q, sort, limit, offset)
}

// Address operations
func (s *Service) ListAddresses(ctx context.Context, brandID uuid.UUID, includePrivate bool) ([]*domain.BrandAddress, error) {
	return s.addresses.List(ctx, brandID, includePrivate)
}

func (s *Service) CreateAddress(ctx context.Context, brandID uuid.UUID, req *domain.CreateAddressRequest) (*domain.BrandAddress, error) {
	if err := s.validateLocation(ctx, req.CityCode, req.DistrictCode, req.WardCode); err != nil {
		return nil, err
	}
	return s.addresses.Create(ctx, brandID, req)
}

func (s *Service) UpdateAddress(ctx context.Context, id, brandID uuid.UUID, req *domain.UpdateAddressRequest) (*domain.BrandAddress, error) {
	cityNil := req.CityCode == nil
	distNil := req.DistrictCode == nil
	wardNil := req.WardCode == nil
	anyNil := cityNil || distNil || wardNil
	allNil := cityNil && distNil && wardNil

	if allNil {
		// No location codes provided — skip validation, preserve existing codes.
	} else if anyNil {
		// Partial location update is not allowed — all three or none.
		return nil, domain.ErrInvalidLocation
	} else {
		// All three provided — validate hierarchy.
		if err := s.validateLocation(ctx, req.CityCode, req.DistrictCode, req.WardCode); err != nil {
			return nil, err
		}
	}

	a, err := s.addresses.Update(ctx, id, brandID, req)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, domain.ErrAddressNotFound
	}
	return a, err
}

func (s *Service) DeleteAddress(ctx context.Context, id, brandID uuid.UUID) error {
	err := s.addresses.SoftDelete(ctx, id, brandID)
	if errors.Is(err, repo.ErrNotFound) {
		return domain.ErrAddressNotFound
	}
	return err
}

// validateLocation checks that districtCode belongs to cityCode and wardCode
// belongs to districtCode using the location service.
// All three pointers must be non-nil; returns ErrInvalidLocation if any is nil.
func (s *Service) validateLocation(ctx context.Context, cityCode, districtCode, wardCode *string) error {
	if cityCode == nil || districtCode == nil || wardCode == nil {
		return domain.ErrInvalidLocation
	}
	districts, err := s.loc.Districts(ctx, *cityCode)
	if err != nil {
		return domain.ErrInvalidLocation
	}
	if !containsCode(districts, *districtCode) {
		return domain.ErrInvalidLocation
	}
	wards, err := s.loc.Wards(ctx, *districtCode)
	if err != nil {
		return domain.ErrInvalidLocation
	}
	if !containsCode(wards, *wardCode) {
		return domain.ErrInvalidLocation
	}
	return nil
}

func containsCode(list []goship.Location, code string) bool {
	for _, l := range list {
		if l.Code == code {
			return true
		}
	}
	return false
}
