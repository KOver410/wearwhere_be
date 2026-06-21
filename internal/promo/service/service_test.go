package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/promo/domain"
	"github.com/wearwhere/wearwhere_be/internal/promo/repo"
)

// fakePromoRepo is an in-memory PromoRepo for white-box service tests.
type fakePromoRepo struct {
	byCode    map[string]*domain.PromoCode
	redeemed  map[string]bool // key: promoID|userID
	insertErr error
}

func newFakeRepo() *fakePromoRepo {
	return &fakePromoRepo{byCode: map[string]*domain.PromoCode{}, redeemed: map[string]bool{}}
}

func key(p, u uuid.UUID) string { return p.String() + "|" + u.String() }

func (f *fakePromoRepo) GetActiveByCode(_ context.Context, _ repo.DBTX, code string) (*domain.PromoCode, error) {
	p, ok := f.byCode[code]
	if !ok || !p.IsActive {
		return nil, repo.ErrNotFound
	}
	return p, nil
}

func (f *fakePromoRepo) GetActiveByCodeForUpdate(ctx context.Context, db repo.DBTX, code string) (*domain.PromoCode, error) {
	return f.GetActiveByCode(ctx, db, code)
}

func (f *fakePromoRepo) HasRedeemed(_ context.Context, _ repo.DBTX, promoID, userID uuid.UUID) (bool, error) {
	return f.redeemed[key(promoID, userID)], nil
}

func (f *fakePromoRepo) InsertRedemption(_ context.Context, _ repo.DBTX, promoID, userID, _ uuid.UUID, _ int64) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	k := key(promoID, userID)
	if f.redeemed[k] {
		return repo.ErrAlreadyRedeemed
	}
	f.redeemed[k] = true
	return nil
}

func (f *fakePromoRepo) Create(_ context.Context, p *domain.PromoCode) error {
	if _, ok := f.byCode[p.Code]; ok {
		return repo.ErrCodeConflict
	}
	p.ID = uuid.New()
	f.byCode[p.Code] = p
	return nil
}
func (f *fakePromoRepo) Update(_ context.Context, _ *domain.PromoCode) error { return nil }
func (f *fakePromoRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.PromoCode, error) {
	return nil, repo.ErrNotFound
}
func (f *fakePromoRepo) List(_ context.Context, _, _ int, _ bool) ([]*domain.PromoCode, int, error) {
	return nil, 0, nil
}

// fixedClock returns a service whose clock is pinned to t.
func svcAt(r repo.PromoRepo, t time.Time) *Service {
	s := New(r)
	s.now = func() time.Time { return t }
	return s
}

func i64(v int64) *int64 { return &v }

func TestComputeDiscount(t *testing.T) {
	tests := []struct {
		name     string
		p        domain.PromoCode
		subtotal int64
		want     int64
	}{
		{"percentage 10%", domain.PromoCode{DiscountType: domain.DiscountTypePercentage, DiscountValue: 10}, 200000, 20000},
		{"percentage capped", domain.PromoCode{DiscountType: domain.DiscountTypePercentage, DiscountValue: 50, MaxDiscountVND: i64(30000)}, 200000, 30000},
		{"fixed", domain.PromoCode{DiscountType: domain.DiscountTypeFixed, DiscountValue: 50000}, 200000, 50000},
		{"fixed clamped at subtotal", domain.PromoCode{DiscountType: domain.DiscountTypeFixed, DiscountValue: 300000}, 200000, 200000},
		{"zero subtotal", domain.PromoCode{DiscountType: domain.DiscountTypeFixed, DiscountValue: 50000}, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.p.ComputeDiscount(tt.subtotal))
		})
	}
}

func TestValidate_Success_Percentage(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	r := newFakeRepo()
	r.byCode["GIAM10"] = &domain.PromoCode{
		ID: uuid.New(), Code: "GIAM10", DiscountType: domain.DiscountTypePercentage,
		DiscountValue: 10, IsActive: true, StartsAt: now.Add(-time.Hour),
	}
	s := svcAt(r, now)

	res, err := s.Validate(context.Background(), " giam10 ", uuid.New(), 200000)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, int64(20000), res.DiscountVND)
}

func TestValidate_EmptyCode_NoError(t *testing.T) {
	s := svcAt(newFakeRepo(), time.Now())
	res, err := s.Validate(context.Background(), "   ", uuid.New(), 200000)
	require.NoError(t, err)
	assert.Nil(t, res)
}

func TestValidate_Unknown_NotFound(t *testing.T) {
	s := svcAt(newFakeRepo(), time.Now())
	_, err := s.Validate(context.Background(), "NOPE", uuid.New(), 200000)
	assert.ErrorIs(t, err, domain.ErrPromoNotFound)
}

func TestValidate_Inactive_NotFound(t *testing.T) {
	now := time.Now()
	r := newFakeRepo()
	r.byCode["OFF"] = &domain.PromoCode{ID: uuid.New(), Code: "OFF", DiscountType: domain.DiscountTypeFixed, DiscountValue: 50000, IsActive: false, StartsAt: now.Add(-time.Hour)}
	s := svcAt(r, now)
	_, err := s.Validate(context.Background(), "OFF", uuid.New(), 200000)
	assert.ErrorIs(t, err, domain.ErrPromoNotFound)
}

func TestValidate_Expired(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	r := newFakeRepo()
	end := now.Add(-time.Minute)
	r.byCode["OLD"] = &domain.PromoCode{ID: uuid.New(), Code: "OLD", DiscountType: domain.DiscountTypeFixed, DiscountValue: 50000, IsActive: true, StartsAt: now.Add(-time.Hour), EndsAt: &end}
	s := svcAt(r, now)
	_, err := s.Validate(context.Background(), "OLD", uuid.New(), 200000)
	assert.ErrorIs(t, err, domain.ErrPromoExpired)
}

func TestValidate_NotStarted(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	r := newFakeRepo()
	r.byCode["SOON"] = &domain.PromoCode{ID: uuid.New(), Code: "SOON", DiscountType: domain.DiscountTypeFixed, DiscountValue: 50000, IsActive: true, StartsAt: now.Add(time.Hour)}
	s := svcAt(r, now)
	_, err := s.Validate(context.Background(), "SOON", uuid.New(), 200000)
	assert.ErrorIs(t, err, domain.ErrPromoNotStarted)
}

func TestValidate_MinOrder(t *testing.T) {
	now := time.Now()
	r := newFakeRepo()
	r.byCode["BIG"] = &domain.PromoCode{ID: uuid.New(), Code: "BIG", DiscountType: domain.DiscountTypeFixed, DiscountValue: 50000, IsActive: true, StartsAt: now.Add(-time.Hour), MinOrderValueVND: 500000}
	s := svcAt(r, now)
	_, err := s.Validate(context.Background(), "BIG", uuid.New(), 200000)
	assert.ErrorIs(t, err, domain.ErrPromoMinOrder)
}

func TestValidate_AlreadyUsed(t *testing.T) {
	now := time.Now()
	r := newFakeRepo()
	pid := uuid.New()
	uid := uuid.New()
	r.byCode["ONCE"] = &domain.PromoCode{ID: pid, Code: "ONCE", DiscountType: domain.DiscountTypeFixed, DiscountValue: 50000, IsActive: true, StartsAt: now.Add(-time.Hour)}
	r.redeemed[key(pid, uid)] = true
	s := svcAt(r, now)
	_, err := s.Validate(context.Background(), "ONCE", uid, 200000)
	assert.ErrorIs(t, err, domain.ErrPromoAlreadyUsed)
}

func TestRedeemTx_RaceSurfacesAlreadyUsed(t *testing.T) {
	r := newFakeRepo()
	pid, uid := uuid.New(), uuid.New()
	s := svcAt(r, time.Now())
	require.NoError(t, s.RedeemTx(context.Background(), nil, pid, uid, uuid.New(), 1000))
	err := s.RedeemTx(context.Background(), nil, pid, uid, uuid.New(), 1000)
	assert.ErrorIs(t, err, domain.ErrPromoAlreadyUsed)
}

func TestCreateCode_Validation(t *testing.T) {
	s := svcAt(newFakeRepo(), time.Now())
	// percentage out of range
	_, err := s.CreateCode(context.Background(), domain.CreatePromoReq{Code: "X", DiscountType: domain.DiscountTypePercentage, DiscountValue: 150})
	assert.ErrorIs(t, err, domain.ErrInvalidPromo)
	// missing type
	_, err = s.CreateCode(context.Background(), domain.CreatePromoReq{Code: "Y", DiscountValue: 10})
	assert.ErrorIs(t, err, domain.ErrInvalidPromo)
	// valid
	p, err := s.CreateCode(context.Background(), domain.CreatePromoReq{Code: "good", DiscountType: domain.DiscountTypeFixed, DiscountValue: 50000})
	require.NoError(t, err)
	assert.Equal(t, "GOOD", p.Code)
}

func TestCreateCode_Duplicate(t *testing.T) {
	s := svcAt(newFakeRepo(), time.Now())
	_, err := s.CreateCode(context.Background(), domain.CreatePromoReq{Code: "DUP", DiscountType: domain.DiscountTypeFixed, DiscountValue: 1000})
	require.NoError(t, err)
	_, err = s.CreateCode(context.Background(), domain.CreatePromoReq{Code: "dup", DiscountType: domain.DiscountTypeFixed, DiscountValue: 1000})
	assert.ErrorIs(t, err, domain.ErrPromoCodeExists)
}
