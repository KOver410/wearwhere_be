package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/review/domain"
	"github.com/wearwhere/wearwhere_be/internal/review/repo"
)

type fakeRepo struct {
	productExists bool
	delivered     bool
	createErr     error
	created       *domain.Review
	getByID       *domain.Review
	updateErr     error
	deleteErr     error
}

func (f *fakeRepo) ProductExists(context.Context, uuid.UUID) (bool, error) { return f.productExists, nil }
func (f *fakeRepo) HasDeliveredPurchase(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return f.delivered, nil
}
func (f *fakeRepo) Create(_ context.Context, r *domain.Review) error {
	if f.createErr != nil {
		return f.createErr
	}
	r.ID = uuid.New()
	f.created = r
	return nil
}
func (f *fakeRepo) GetByID(context.Context, uuid.UUID) (*domain.Review, error) {
	if f.getByID == nil {
		return nil, repo.ErrNotFound
	}
	return f.getByID, nil
}
func (f *fakeRepo) Update(context.Context, uuid.UUID, int, string, *string) error { return f.updateErr }
func (f *fakeRepo) SoftDelete(context.Context, uuid.UUID) error                   { return f.deleteErr }
func (f *fakeRepo) ListByProduct(context.Context, uuid.UUID, repo.ListFilter) ([]*domain.ReviewView, int, error) {
	return nil, 0, nil
}
func (f *fakeRepo) Aggregate(context.Context, uuid.UUID) (repo.Aggregate, error) {
	return repo.Aggregate{}, nil
}

func newSvc(f *fakeRepo) *Service { return NewWithRepo(f) }

func TestCreate_RejectsUnverifiedPurchase(t *testing.T) {
	svc := newSvc(&fakeRepo{productExists: true, delivered: false})
	_, err := svc.Create(context.Background(), uuid.New(), uuid.New(),
		&domain.WriteReviewRequest{Rating: 5, Body: "Twenty characters minimum body!!"})
	if err == nil {
		t.Fatal("expected NOT_VERIFIED_PURCHASE error")
	}
}

func TestCreate_RejectsMissingProduct(t *testing.T) {
	svc := newSvc(&fakeRepo{productExists: false})
	_, err := svc.Create(context.Background(), uuid.New(), uuid.New(),
		&domain.WriteReviewRequest{Rating: 5, Body: "Twenty characters minimum body!!"})
	if err == nil {
		t.Fatal("expected PRODUCT_NOT_FOUND error")
	}
}

func TestCreate_DuplicateMapsToReviewExists(t *testing.T) {
	svc := newSvc(&fakeRepo{productExists: true, delivered: true, createErr: repo.ErrDuplicate})
	_, err := svc.Create(context.Background(), uuid.New(), uuid.New(),
		&domain.WriteReviewRequest{Rating: 5, Body: "Twenty characters minimum body!!"})
	if err == nil {
		t.Fatal("expected REVIEW_EXISTS error")
	}
}

func TestCreate_Success(t *testing.T) {
	f := &fakeRepo{productExists: true, delivered: true}
	svc := newSvc(f)
	got, err := svc.Create(context.Background(), uuid.New(), uuid.New(),
		&domain.WriteReviewRequest{Rating: 4, Body: "Twenty characters minimum body!!", Fit: "true"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Rating != 4 || f.created == nil {
		t.Errorf("review not created as expected: %+v", got)
	}
	if got.Fit == nil || *got.Fit != "true" {
		t.Errorf("expected fit=true, got %v", got.Fit)
	}
}

func TestUpdate_RejectsNonOwner(t *testing.T) {
	owner := uuid.New()
	rv := &domain.Review{ID: uuid.New(), UserID: owner, ProductID: uuid.New()}
	svc := newSvc(&fakeRepo{getByID: rv})
	err := svc.Update(context.Background(), uuid.New(), rv.ID,
		&domain.WriteReviewRequest{Rating: 3, Body: "Twenty characters minimum body!!"})
	if err == nil {
		t.Fatal("expected FORBIDDEN for non-owner update")
	}
}

func TestDelete_RejectsNonOwner(t *testing.T) {
	owner := uuid.New()
	rv := &domain.Review{ID: uuid.New(), UserID: owner, ProductID: uuid.New()}
	svc := newSvc(&fakeRepo{getByID: rv})
	err := svc.Delete(context.Background(), uuid.New(), rv.ID)
	if err == nil {
		t.Fatal("expected FORBIDDEN for non-owner delete")
	}
}
