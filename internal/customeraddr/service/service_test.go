package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
	"github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
	"github.com/wearwhere/wearwhere_be/internal/customeraddr/service"
)

type mockRepo struct {
	findErr     error
	deleteErr   error
	list        []*domain.CustomerAddress
	findReturns *domain.CustomerAddress
}

func (m *mockRepo) List(_ context.Context, _ uuid.UUID) ([]*domain.CustomerAddress, error) {
	return m.list, nil
}
func (m *mockRepo) FindByID(_ context.Context, _, _ uuid.UUID) (*domain.CustomerAddress, error) {
	return m.findReturns, m.findErr
}
func (m *mockRepo) Create(_ context.Context, _ uuid.UUID, _ *domain.CreateAddressRequest) (*domain.CustomerAddress, error) {
	return &domain.CustomerAddress{IsDefault: true}, nil
}
func (m *mockRepo) Update(_ context.Context, _, _ uuid.UUID, _ *domain.UpdateAddressRequest) (*domain.CustomerAddress, error) {
	return m.findReturns, m.findErr
}
func (m *mockRepo) SoftDelete(_ context.Context, _, _ uuid.UUID) error { return m.deleteErr }

func TestGet_NotFoundMapsToDomainError(t *testing.T) {
	s := service.New(&mockRepo{findErr: repo.ErrNotFound})
	_, err := s.Get(context.Background(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, domain.ErrAddressNotFound)
}

func TestDelete_NotFoundMapsToDomainError(t *testing.T) {
	s := service.New(&mockRepo{deleteErr: repo.ErrNotFound})
	err := s.Delete(context.Background(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, domain.ErrAddressNotFound)
}

func TestCreate_PassesThroughRepo(t *testing.T) {
	s := service.New(&mockRepo{})
	a, err := s.Create(context.Background(), uuid.New(), &domain.CreateAddressRequest{Label: "Nhà"})
	require.NoError(t, err)
	require.True(t, a.IsDefault)
}
