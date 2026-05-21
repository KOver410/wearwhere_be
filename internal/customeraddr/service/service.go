// Package service implements customer-address business rules.
package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
	"github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
)

type CustomerAddressService struct {
	repo repo.AddressRepo
}

func New(r repo.AddressRepo) *CustomerAddressService { return &CustomerAddressService{repo: r} }

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
	return s.repo.Create(ctx, userID, req)
}

func (s *CustomerAddressService) Update(ctx context.Context, id, userID uuid.UUID, req *domain.UpdateAddressRequest) (*domain.CustomerAddress, error) {
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
