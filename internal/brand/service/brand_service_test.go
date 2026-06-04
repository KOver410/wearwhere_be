package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/brand/domain"
	"github.com/wearwhere/wearwhere_be/internal/brand/repo"
	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
	"github.com/wearwhere/wearwhere_be/internal/shipping/location"
)

// fakeBrandRepo — minimal in-memory impl for unit tests.
type fakeBrandRepo struct {
	byID      map[uuid.UUID]*domain.Brand
	byOwner   map[uuid.UUID]*domain.Brand
	updateErr error
}

func (f *fakeBrandRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Brand, error) {
	b, ok := f.byID[id]
	if !ok {
		return nil, repo.ErrNotFound
	}
	return b, nil
}
func (f *fakeBrandRepo) FindBySlug(ctx context.Context, slug string) (*domain.Brand, error) {
	for _, b := range f.byID {
		if b.Slug == slug {
			return b, nil
		}
	}
	return nil, repo.ErrNotFound
}
func (f *fakeBrandRepo) FindByOwnerUserID(ctx context.Context, uid uuid.UUID) (*domain.Brand, error) {
	b, ok := f.byOwner[uid]
	if !ok {
		return nil, repo.ErrNotFound
	}
	return b, nil
}
func (f *fakeBrandRepo) Update(ctx context.Context, id uuid.UUID, req *domain.UpdateBrandRequest) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	b, ok := f.byID[id]
	if !ok {
		return repo.ErrNotFound
	}
	if req.Name != nil {
		b.Name = *req.Name
	}
	if req.Slug != nil {
		b.Slug = *req.Slug
	}
	return nil
}
func (f *fakeBrandRepo) List(ctx context.Context, q, sort string, limit, offset int) ([]*domain.Brand, int, error) {
	return nil, 0, nil
}

type fakeAddrRepo struct{}

func (f *fakeAddrRepo) List(ctx context.Context, brandID uuid.UUID, includePrivate bool) ([]*domain.BrandAddress, error) {
	return nil, nil
}
func (f *fakeAddrRepo) FindByID(ctx context.Context, id, brandID uuid.UUID) (*domain.BrandAddress, error) {
	return nil, repo.ErrNotFound
}
func (f *fakeAddrRepo) Create(ctx context.Context, brandID uuid.UUID, req *domain.CreateAddressRequest) (*domain.BrandAddress, error) {
	return nil, nil
}
func (f *fakeAddrRepo) Update(ctx context.Context, id, brandID uuid.UUID, req *domain.UpdateAddressRequest) (*domain.BrandAddress, error) {
	return nil, repo.ErrNotFound
}
func (f *fakeAddrRepo) SoftDelete(ctx context.Context, id, brandID uuid.UUID) error {
	return repo.ErrNotFound
}

func newTestLocSvc() *location.Service {
	return location.NewService(goship.NewMockClient(), 24*time.Hour)
}

func TestService_GetByID_NotFound_Translates(t *testing.T) {
	svc := New(&fakeBrandRepo{byID: map[uuid.UUID]*domain.Brand{}, byOwner: map[uuid.UUID]*domain.Brand{}}, &fakeAddrRepo{}, newTestLocSvc())
	_, err := svc.GetByID(context.Background(), uuid.New())
	require.ErrorIs(t, err, domain.ErrBrandNotFound)
}

func TestService_UpdateOwn_SlugConflict_Translates(t *testing.T) {
	id := uuid.New()
	svc := New(
		&fakeBrandRepo{
			byID:      map[uuid.UUID]*domain.Brand{id: {ID: id}},
			byOwner:   map[uuid.UUID]*domain.Brand{},
			updateErr: repo.ErrSlugTaken,
		},
		&fakeAddrRepo{},
		newTestLocSvc(),
	)
	newSlug := "taken"
	err := svc.UpdateOwn(context.Background(), id, &domain.UpdateBrandRequest{Slug: &newSlug})
	require.ErrorIs(t, err, domain.ErrSlugTaken)
}

func TestService_DeleteAddress_NotFound_Translates(t *testing.T) {
	svc := New(&fakeBrandRepo{}, &fakeAddrRepo{}, newTestLocSvc())
	err := svc.DeleteAddress(context.Background(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, domain.ErrAddressNotFound)
}

// silence unused
var _ = errors.New
