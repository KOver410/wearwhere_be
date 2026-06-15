package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/block/domain"
)

type fakeRepo struct {
	userExists bool
	blocked    [][2]uuid.UUID
}

func (f *fakeRepo) UserExists(context.Context, uuid.UUID) (bool, error) { return f.userExists, nil }
func (f *fakeRepo) Block(_ context.Context, a, b uuid.UUID) error {
	f.blocked = append(f.blocked, [2]uuid.UUID{a, b})
	return nil
}
func (f *fakeRepo) Unblock(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (f *fakeRepo) ListBlocked(context.Context, uuid.UUID, int, int) ([]domain.BlockedUserItem, int, error) {
	return nil, 0, nil
}

func TestBlockUser_RejectsSelf(t *testing.T) {
	svc := New(&fakeRepo{userExists: true})
	id := uuid.New()
	if _, err := svc.BlockUser(context.Background(), id, id); err == nil {
		t.Fatal("expected CANNOT_BLOCK_SELF")
	}
}

func TestBlockUser_RejectsMissingTarget(t *testing.T) {
	svc := New(&fakeRepo{userExists: false})
	if _, err := svc.BlockUser(context.Background(), uuid.New(), uuid.New()); err == nil {
		t.Fatal("expected USER_NOT_FOUND")
	}
}

func TestBlockUser_Success(t *testing.T) {
	f := &fakeRepo{userExists: true}
	svc := New(f)
	resp, err := svc.BlockUser(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !resp.Blocked || len(f.blocked) != 1 {
		t.Errorf("got resp=%+v writes=%d", resp, len(f.blocked))
	}
}

func TestUnblockUser_Success(t *testing.T) {
	svc := New(&fakeRepo{})
	resp, err := svc.UnblockUser(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if resp.Blocked {
		t.Errorf("got %+v, want Blocked=false", resp)
	}
}
