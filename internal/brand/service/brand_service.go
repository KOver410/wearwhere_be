package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/brand/domain"
	"github.com/wearwhere/wearwhere_be/internal/brand/repo"
)

type Service struct {
	brands    repo.BrandRepo
	addresses repo.AddressRepo
}

func New(b repo.BrandRepo, a repo.AddressRepo) *Service {
	return &Service{brands: b, addresses: a}
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
	return s.addresses.Create(ctx, brandID, req)
}

func (s *Service) UpdateAddress(ctx context.Context, id, brandID uuid.UUID, req *domain.UpdateAddressRequest) (*domain.BrandAddress, error) {
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
