// Package service implements customer-address business rules.
package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
	"github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
	"github.com/wearwhere/wearwhere_be/internal/shipping/location"
)

type CustomerAddressService struct {
	repo repo.AddressRepo
	loc  *location.Service
}

func New(r repo.AddressRepo, loc *location.Service) *CustomerAddressService {
	return &CustomerAddressService{repo: r, loc: loc}
}

func (s *CustomerAddressService) List(ctx context.Context, userID uuid.UUID) ([]*domain.CustomerAddress, error) {
	return s.repo.List(ctx, userID)
}

func (s *CustomerAddressService) Get(ctx context.Context, id, userID uuid.UUID) (*domain.CustomerAddress, error) {
	a, err := s.repo.FindByID(ctx, id, userID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, domain.ErrAddressNotFound
	}
	return a, err
}

func (s *CustomerAddressService) Create(ctx context.Context, userID uuid.UUID, req *domain.CreateAddressRequest) (*domain.CustomerAddress, error) {
	if err := s.validateLocation(ctx, req.CityCode, req.DistrictCode, req.WardCode); err != nil {
		return nil, err
	}
	return s.repo.Create(ctx, userID, req)
}

func (s *CustomerAddressService) Update(ctx context.Context, id, userID uuid.UUID, req *domain.UpdateAddressRequest) (*domain.CustomerAddress, error) {
	if err := s.validateLocation(ctx, req.CityCode, req.DistrictCode, req.WardCode); err != nil {
		return nil, err
	}
	a, err := s.repo.Update(ctx, id, userID, req)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, domain.ErrAddressNotFound
	}
	return a, err
}

func (s *CustomerAddressService) Delete(ctx context.Context, id, userID uuid.UUID) error {
	if err := s.repo.SoftDelete(ctx, id, userID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.ErrAddressNotFound
		}
		return err
	}
	return nil
}

// validateLocation checks that districtCode belongs to cityCode and wardCode
// belongs to districtCode using the location service.
func (s *CustomerAddressService) validateLocation(ctx context.Context, cityCode, districtCode, wardCode *string) error {
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
