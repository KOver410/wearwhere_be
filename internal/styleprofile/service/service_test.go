package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/repo"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/service"
)

type fakeRepo struct {
	loadErr  error
	view     *domain.StyleProfileView
	unknown  []uuid.UUID
	upserted *domain.UpsertParams
}

func (f *fakeRepo) Load(ctx context.Context, userID uuid.UUID) (*domain.StyleProfileView, error) {
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	return f.view, nil
}
func (f *fakeRepo) Upsert(ctx context.Context, p domain.UpsertParams) (*domain.StyleProfileView, error) {
	f.upserted = &p
	return &domain.StyleProfileView{UserID: p.UserID, BudgetMin: p.BudgetMin, BudgetMax: p.BudgetMax}, nil
}
func (f *fakeRepo) UnknownTagIDs(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error) {
	return f.unknown, nil
}

func intp(v int) *int { return &v }

func TestGet_EmptyWhenNoProfile(t *testing.T) {
	svc := service.New(&fakeRepo{loadErr: repo.ErrNotFound})
	uid := uuid.New()
	v, err := svc.Get(context.Background(), uid)
	require.NoError(t, err)
	require.Equal(t, uid, v.UserID)
	require.Empty(t, v.StyleTags)
	require.Nil(t, v.OnboardedAt)
}

func TestSave_RejectsBadBudget(t *testing.T) {
	svc := service.New(&fakeRepo{})
	_, err := svc.Save(context.Background(), uuid.New(), domain.UpdateStyleProfileRequest{
		BudgetMin: intp(500000), BudgetMax: intp(100000),
	})
	require.ErrorIs(t, err, domain.ErrInvalidBudget)
}

func TestSave_RejectsUnknownTags(t *testing.T) {
	bad := uuid.New()
	svc := service.New(&fakeRepo{unknown: []uuid.UUID{bad}})
	_, err := svc.Save(context.Background(), uuid.New(), domain.UpdateStyleProfileRequest{
		StyleTagIDs: []string{bad.String()},
	})
	var ute *domain.UnknownStyleTagsError
	require.ErrorAs(t, err, &ute)
	require.Equal(t, []string{bad.String()}, ute.IDs)
}

func TestSave_InvalidUUIDInTagsIsRejected(t *testing.T) {
	svc := service.New(&fakeRepo{})
	_, err := svc.Save(context.Background(), uuid.New(), domain.UpdateStyleProfileRequest{
		StyleTagIDs: []string{"not-a-uuid"},
	})
	require.Error(t, err)
}

func TestSave_PassesParsedParamsToRepo(t *testing.T) {
	tag := uuid.New()
	f := &fakeRepo{}
	svc := service.New(f)
	_, err := svc.Save(context.Background(), uuid.New(), domain.UpdateStyleProfileRequest{
		StyleTagIDs: []string{tag.String()}, BudgetMin: intp(100000),
	})
	require.NoError(t, err)
	require.NotNil(t, f.upserted)
	require.Equal(t, []uuid.UUID{tag}, f.upserted.StyleTagIDs)
	require.Equal(t, 100000, *f.upserted.BudgetMin)
}
