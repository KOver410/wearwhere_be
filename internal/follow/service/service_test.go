package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/follow/domain"
)

type fakeRepo struct {
	userExists, brandExists bool
	count                   int
}

func (f *fakeRepo) UserExists(context.Context, uuid.UUID) (bool, error)  { return f.userExists, nil }
func (f *fakeRepo) BrandExists(context.Context, uuid.UUID) (bool, error) { return f.brandExists, nil }
func (f *fakeRepo) FollowUser(context.Context, uuid.UUID, uuid.UUID) (int, error)   { return f.count, nil }
func (f *fakeRepo) UnfollowUser(context.Context, uuid.UUID, uuid.UUID) (int, error) { return f.count, nil }
func (f *fakeRepo) FollowBrand(context.Context, uuid.UUID, uuid.UUID) (int, error)  { return f.count, nil }
func (f *fakeRepo) UnfollowBrand(context.Context, uuid.UUID, uuid.UUID) (int, error) { return f.count, nil }
func (f *fakeRepo) ListFollowingUsers(context.Context, uuid.UUID, int, int) ([]domain.FollowingUserItem, int, error) {
	return nil, 0, nil
}
func (f *fakeRepo) ListFollowingBrands(context.Context, uuid.UUID, int, int) ([]domain.FollowingBrandItem, int, error) {
	return nil, 0, nil
}

func TestFollowUser_RejectsSelf(t *testing.T) {
	svc := New(&fakeRepo{userExists: true})
	id := uuid.New()
	if _, err := svc.FollowUser(context.Background(), id, id); err == nil {
		t.Fatal("expected CANNOT_FOLLOW_SELF")
	}
}

func TestFollowUser_RejectsMissingTarget(t *testing.T) {
	svc := New(&fakeRepo{userExists: false})
	if _, err := svc.FollowUser(context.Background(), uuid.New(), uuid.New()); err == nil {
		t.Fatal("expected USER_NOT_FOUND")
	}
}

func TestFollowUser_Success(t *testing.T) {
	svc := New(&fakeRepo{userExists: true, count: 3})
	resp, err := svc.FollowUser(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !resp.Following || resp.FollowerCount != 3 {
		t.Errorf("got %+v", resp)
	}
}

func TestFollowBrand_RejectsMissing(t *testing.T) {
	svc := New(&fakeRepo{brandExists: false})
	if _, err := svc.FollowBrand(context.Background(), uuid.New(), uuid.New()); err == nil {
		t.Fatal("expected BRAND_NOT_FOUND")
	}
}

func TestUnfollowBrand_Success(t *testing.T) {
	svc := New(&fakeRepo{brandExists: true, count: 0})
	resp, err := svc.UnfollowBrand(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if resp.Following || resp.FollowerCount != 0 {
		t.Errorf("got %+v", resp)
	}
}
