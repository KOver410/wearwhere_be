package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
	"github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
	"github.com/wearwhere/wearwhere_be/internal/customeraddr/service"
	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
	"github.com/wearwhere/wearwhere_be/internal/shipping/location"
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

// newTestLocSvc creates a location.Service backed by the mock goship client.
func newTestLocSvc() *location.Service {
	return location.NewService(goship.NewMockClient(), 24*time.Hour)
}

// validCodes returns pointer-coded valid codes matching the mock hierarchy.
// City "100000" → district "100000100" → ward "10000010001".
func sp(s string) *string { return &s }

func TestGet_NotFoundMapsToDomainError(t *testing.T) {
	s := service.New(&mockRepo{findErr: repo.ErrNotFound}, newTestLocSvc())
	_, err := s.Get(context.Background(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, domain.ErrAddressNotFound)
}

func TestDelete_NotFoundMapsToDomainError(t *testing.T) {
	s := service.New(&mockRepo{deleteErr: repo.ErrNotFound}, newTestLocSvc())
	err := s.Delete(context.Background(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, domain.ErrAddressNotFound)
}

func TestCreate_PassesThroughRepo(t *testing.T) {
	s := service.New(&mockRepo{}, newTestLocSvc())
	a, err := s.Create(context.Background(), uuid.New(), &domain.CreateAddressRequest{
		Label:          "Nhà",
		RecipientName:  "Nguyễn A",
		RecipientPhone: "+84901234567",
		AddressLine:    "123 Đường ABC",
		Ward:           "Phường 1",
		District:       "Quận 1",
		City:           "Hồ Chí Minh",
		CityCode:       sp("100000"),
		DistrictCode:   sp("100000100"),
		WardCode:       sp("10000010001"),
	})
	require.NoError(t, err)
	require.True(t, a.IsDefault)
}

func TestCreateAddress_RejectsInconsistentCodes(t *testing.T) {
	s := service.New(&mockRepo{}, newTestLocSvc())
	_, err := s.Create(context.Background(), uuid.New(), &domain.CreateAddressRequest{
		Label:          "Nhà",
		RecipientName:  "Nguyễn A",
		RecipientPhone: "+84901234567",
		AddressLine:    "123 Đường ABC",
		Ward:           "Phường 1",
		District:       "Quận 999",
		City:           "Hồ Chí Minh",
		CityCode:       sp("100000"),
		DistrictCode:   sp("999"),   // not in mock's districts for city 100000
		WardCode:       sp("99901"),
	})
	require.ErrorIs(t, err, domain.ErrInvalidLocation)
}

func TestCreateAddress_ValidCodes_Succeeds(t *testing.T) {
	s := service.New(&mockRepo{}, newTestLocSvc())
	// mock: city 100000 → district 100000100 → ward 10000010001
	a, err := s.Create(context.Background(), uuid.New(), &domain.CreateAddressRequest{
		Label:          "Văn phòng",
		RecipientName:  "Trần B",
		RecipientPhone: "+84901234567",
		AddressLine:    "456 Đường XYZ",
		Ward:           "Phường 1",
		District:       "Quận 1",
		City:           "Hồ Chí Minh",
		CityCode:       sp("100000"),
		DistrictCode:   sp("100000100"),
		WardCode:       sp("10000010001"),
	})
	require.NoError(t, err)
	require.True(t, a.IsDefault)
}
