package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/product/domain"
	"github.com/wearwhere/wearwhere_be/internal/product/repo"
)

// fakeProductRepo: just enough for slug tests
type fakeProductRepo struct {
	existingSlugs map[string]bool
	createCalled  bool
	createdSlug   string
}

func (f *fakeProductRepo) Create(ctx context.Context, brandID uuid.UUID, slug string, req *domain.CreateProductRequest) (*domain.Product, error) {
	f.createCalled = true
	f.createdSlug = slug
	return &domain.Product{ID: uuid.New(), BrandID: brandID, Slug: slug, Name: req.Name, Status: domain.ProductStatusDraft}, nil
}
func (f *fakeProductRepo) SlugExists(ctx context.Context, brandID uuid.UUID, slug string) (bool, error) {
	return f.existingSlugs[slug], nil
}
func (f *fakeProductRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Product, error) {
	return nil, repo.ErrNotFound
}
func (f *fakeProductRepo) FindByBrandSlug(ctx context.Context, bs, ps string) (*domain.Product, error) {
	return nil, repo.ErrNotFound
}
func (f *fakeProductRepo) Update(ctx context.Context, id, brandID uuid.UUID, r *domain.UpdateProductRequest) error {
	return nil
}
func (f *fakeProductRepo) SoftDelete(ctx context.Context, id, brandID uuid.UUID) error { return nil }
func (f *fakeProductRepo) ListByBrand(ctx context.Context, brandID uuid.UUID, l, o int) ([]*domain.Product, int, error) {
	return nil, 0, nil
}
func (f *fakeProductRepo) IncrementViewCount(ctx context.Context, id uuid.UUID) error { return nil }
func (f *fakeProductRepo) SetStyleTags(ctx context.Context, p uuid.UUID, ids []uuid.UUID) error {
	return nil
}
func (f *fakeProductRepo) GetStyleTags(ctx context.Context, p uuid.UUID) ([]*domain.StyleTag, error) {
	return nil, nil
}

func TestService_SlugFromName_AppendsSuffixOnCollision(t *testing.T) {
	fr := &fakeProductRepo{existingSlugs: map[string]bool{"ao-thun-trang": true, "ao-thun-trang-2": true}}
	svc := New(fr, nil, nil, nil, nil, nil, nil, 0)
	bid := uuid.New()
	cid := uuid.New().String()
	_, err := svc.CreateProduct(context.Background(), bid, &domain.CreateProductRequest{
		Name: "Áo Thun Trắng", CategoryID: cid,
	})
	require.NoError(t, err)
	require.True(t, fr.createCalled)
	require.Equal(t, "ao-thun-trang-3", fr.createdSlug)
}

func TestService_ExplicitSlug_RejectsConflict(t *testing.T) {
	fr := &fakeProductRepo{existingSlugs: map[string]bool{"taken": true}}
	svc := New(fr, nil, nil, nil, nil, nil, nil, 0)
	_, err := svc.CreateProduct(context.Background(), uuid.New(), &domain.CreateProductRequest{
		Name: "x", Slug: "taken", CategoryID: uuid.New().String(),
	})
	require.ErrorIs(t, err, domain.ErrSlugTaken)
}

// fakeVariantRepo implements repo.VariantRepo with no-op methods.
type fakeVariantRepo struct {
	variants []*domain.Variant
}

func (f *fakeVariantRepo) Create(ctx context.Context, productID uuid.UUID, req *domain.CreateVariantRequest) (*domain.Variant, error) {
	return nil, nil
}
func (f *fakeVariantRepo) FindByID(ctx context.Context, id, productID uuid.UUID) (*domain.Variant, error) {
	return nil, repo.ErrNotFound
}
func (f *fakeVariantRepo) ListByProduct(ctx context.Context, productID uuid.UUID, onlyActive bool) ([]*domain.Variant, error) {
	return f.variants, nil
}
func (f *fakeVariantRepo) Update(ctx context.Context, id, productID uuid.UUID, req *domain.UpdateVariantRequest) (*domain.Variant, error) {
	return nil, nil
}
func (f *fakeVariantRepo) SoftDelete(ctx context.Context, id, productID uuid.UUID) error { return nil }
func (f *fakeVariantRepo) FindForPurchase(_ context.Context, _ uuid.UUID) (*domain.Variant, *domain.Product, error) {
	return nil, nil, repo.ErrNotFound
}

// fakeImageRepo implements repo.ImageRepo with no-op methods.
type fakeImageRepo struct {
	images []*domain.Image
}

func (f *fakeImageRepo) Create(ctx context.Context, productID uuid.UUID, url, storageKey string) (*domain.Image, error) {
	return nil, nil
}
func (f *fakeImageRepo) FindByID(ctx context.Context, id, productID uuid.UUID) (*domain.Image, error) {
	return nil, repo.ErrNotFound
}
func (f *fakeImageRepo) ListByProduct(ctx context.Context, productID uuid.UUID) ([]*domain.Image, error) {
	return f.images, nil
}
func (f *fakeImageRepo) Update(ctx context.Context, id, productID uuid.UUID, req *domain.UpdateImageRequest) (*domain.Image, error) {
	return nil, nil
}
func (f *fakeImageRepo) Delete(ctx context.Context, id, productID uuid.UUID) (string, bool, error) {
	return "", false, repo.ErrNotFound
}
func (f *fakeImageRepo) PromoteNextPrimary(ctx context.Context, productID uuid.UUID) error {
	return nil
}

// productRepoWithBrand returns a fakeProductRepo where FindByID returns a product
// with the given brandID (used for ownership tests).
type fakeProductRepoWithProduct struct {
	fakeProductRepo
	product *domain.Product
}

func (f *fakeProductRepoWithProduct) FindByID(ctx context.Context, id uuid.UUID) (*domain.Product, error) {
	if f.product != nil {
		return f.product, nil
	}
	return nil, repo.ErrNotFound
}

func TestService_UpdateProduct_BlocksPublishWithNoVariants(t *testing.T) {
	pid := uuid.New()
	bid := uuid.New()
	pr := &fakeProductRepoWithProduct{
		fakeProductRepo: fakeProductRepo{existingSlugs: map[string]bool{}},
		product: &domain.Product{
			ID: pid, BrandID: bid, Slug: "some-product", Name: "Test", Status: domain.ProductStatusDraft,
		},
	}
	vr := &fakeVariantRepo{variants: nil} // no variants
	ir := &fakeImageRepo{images: []*domain.Image{{ID: uuid.New()}}}

	svc := New(pr, vr, ir, nil, nil, nil, nil, 0)
	status := string(domain.ProductStatusActive)
	err := svc.UpdateProduct(context.Background(), pid, bid, &domain.UpdateProductRequest{
		Status: &status,
	})
	require.ErrorIs(t, err, domain.ErrProductNotPublishable)
}

func TestService_UpdateProduct_BlocksPublishWithNoImages(t *testing.T) {
	pid := uuid.New()
	bid := uuid.New()
	pr := &fakeProductRepoWithProduct{
		fakeProductRepo: fakeProductRepo{existingSlugs: map[string]bool{}},
		product: &domain.Product{
			ID: pid, BrandID: bid, Slug: "some-product", Name: "Test", Status: domain.ProductStatusDraft,
		},
	}
	vr := &fakeVariantRepo{variants: []*domain.Variant{{ID: uuid.New()}}} // has variants
	ir := &fakeImageRepo{images: nil}                                     // no images

	svc := New(pr, vr, ir, nil, nil, nil, nil, 0)
	status := string(domain.ProductStatusActive)
	err := svc.UpdateProduct(context.Background(), pid, bid, &domain.UpdateProductRequest{
		Status: &status,
	})
	require.ErrorIs(t, err, domain.ErrProductNotPublishable)
}

func TestService_GetOwnProduct_IDORProtection(t *testing.T) {
	pid := uuid.New()
	realBrand := uuid.New()
	otherBrand := uuid.New()

	pr := &fakeProductRepoWithProduct{
		fakeProductRepo: fakeProductRepo{},
		product:         &domain.Product{ID: pid, BrandID: realBrand, Slug: "p", Name: "P", Status: domain.ProductStatusDraft},
	}
	svc := New(pr, nil, nil, nil, nil, nil, nil, 0)

	// Same brand can access
	p, err := svc.GetOwnProduct(context.Background(), pid, realBrand)
	require.NoError(t, err)
	require.Equal(t, pid, p.ID)

	// Other brand gets not found (IDOR protection)
	_, err = svc.GetOwnProduct(context.Background(), pid, otherBrand)
	require.ErrorIs(t, err, domain.ErrProductNotFound)
}

// alwaysFullRepo is a fakeProductRepo whose SlugExists always returns true,
// simulating total slug-namespace exhaustion.
type alwaysFullRepo struct {
	fakeProductRepo
	createCalled bool
}

func (f *alwaysFullRepo) SlugExists(_ context.Context, _ uuid.UUID, _ string) (bool, error) {
	return true, nil
}
func (f *alwaysFullRepo) Create(_ context.Context, _ uuid.UUID, _ string, _ *domain.CreateProductRequest) (*domain.Product, error) {
	f.createCalled = true
	return &domain.Product{}, nil
}

func TestService_SlugExhaustion_ReturnsErrSlugTaken(t *testing.T) {
	fr := &alwaysFullRepo{}
	svc := New(fr, nil, nil, nil, nil, nil, nil, 0)

	_, err := svc.CreateProduct(context.Background(), uuid.New(), &domain.CreateProductRequest{
		Name: "Packed Product", CategoryID: uuid.New().String(),
	})
	require.ErrorIs(t, err, domain.ErrSlugTaken, "exhausted slug space must return ErrSlugTaken")
	require.False(t, fr.createCalled, "Create must NOT be called when slug space is exhausted")
}
