package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/product/domain"
	"github.com/wearwhere/wearwhere_be/internal/product/repo"
	"github.com/wearwhere/wearwhere_be/internal/shared/slug"
	"github.com/wearwhere/wearwhere_be/internal/shared/storage"
)

type Service struct {
	products     repo.ProductRepo
	variants     repo.VariantRepo
	images       repo.ImageRepo
	categories   repo.CategoryRepo
	styleTags    repo.StyleTagRepo
	storage      storage.Storage
	maxFileSize  int64
	allowedMIMEs map[string]string // mime -> extension
}

func New(
	p repo.ProductRepo, v repo.VariantRepo, i repo.ImageRepo,
	c repo.CategoryRepo, st repo.StyleTagRepo,
	s storage.Storage, allowedMIMEs []string, maxFileSize int64,
) *Service {
	allowed := map[string]string{}
	extMap := map[string]string{
		"image/jpeg": "jpg",
		"image/png":  "png",
		"image/webp": "webp",
	}
	for _, m := range allowedMIMEs {
		if ext, ok := extMap[m]; ok {
			allowed[m] = ext
		}
	}
	if maxFileSize == 0 {
		maxFileSize = 5 * 1024 * 1024
	}
	return &Service{
		products: p, variants: v, images: i,
		categories: c, styleTags: st,
		storage: s, maxFileSize: maxFileSize, allowedMIMEs: allowed,
	}
}

// ── PRODUCT CRUD ──
func (s *Service) CreateProduct(ctx context.Context, brandID uuid.UUID, req *domain.CreateProductRequest) (*domain.Product, error) {
	var theSlug string
	if req.Slug != "" {
		exists, err := s.products.SlugExists(ctx, brandID, req.Slug)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, domain.ErrSlugTaken
		}
		theSlug = req.Slug
	} else {
		base := slug.Slugify(req.Name)
		if base == "" {
			base = "product"
		}
		theSlug = base
		for i := 2; i < 100; i++ {
			exists, err := s.products.SlugExists(ctx, brandID, theSlug)
			if err != nil {
				return nil, err
			}
			if !exists {
				break
			}
			theSlug = fmt.Sprintf("%s-%d", base, i)
		}
	}

	p, err := s.products.Create(ctx, brandID, theSlug, req)
	if err != nil {
		if errors.Is(err, repo.ErrSlugTaken) {
			return nil, domain.ErrSlugTaken
		}
		return nil, err
	}
	if len(req.StyleTagIDs) > 0 {
		ids, err := parseUUIDs(req.StyleTagIDs)
		if err != nil {
			return nil, err
		}
		if err := s.products.SetStyleTags(ctx, p.ID, ids); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (s *Service) GetOwnProduct(ctx context.Context, id, brandID uuid.UUID) (*domain.Product, error) {
	p, err := s.products.FindByID(ctx, id)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, domain.ErrProductNotFound
	}
	if err != nil {
		return nil, err
	}
	if p.BrandID != brandID {
		return nil, domain.ErrProductNotFound
	}
	return p, nil
}

func (s *Service) ListOwnProducts(ctx context.Context, brandID uuid.UUID, limit, offset int) ([]*domain.Product, int, error) {
	return s.products.ListByBrand(ctx, brandID, limit, offset)
}

func (s *Service) UpdateProduct(ctx context.Context, id, brandID uuid.UUID, req *domain.UpdateProductRequest) error {
	// If publishing draft→active, require ≥1 variant and ≥1 image.
	if req.Status != nil && *req.Status == string(domain.ProductStatusActive) {
		variants, err := s.variants.ListByProduct(ctx, id, true)
		if err != nil {
			return err
		}
		images, err := s.images.ListByProduct(ctx, id)
		if err != nil {
			return err
		}
		if len(variants) == 0 || len(images) == 0 {
			return domain.ErrProductNotPublishable
		}
	}
	if req.Slug != nil {
		exists, err := s.products.SlugExists(ctx, brandID, *req.Slug)
		if err != nil {
			return err
		}
		if exists {
			// Allow same slug on this product itself
			p, err := s.products.FindByID(ctx, id)
			if err != nil || p.Slug != *req.Slug {
				return domain.ErrSlugTaken
			}
		}
	}
	err := s.products.Update(ctx, id, brandID, req)
	switch {
	case errors.Is(err, repo.ErrNotFound):
		return domain.ErrProductNotFound
	case errors.Is(err, repo.ErrSlugTaken):
		return domain.ErrSlugTaken
	}
	if err == nil && req.StyleTagIDs != nil {
		ids, perr := parseUUIDs(req.StyleTagIDs)
		if perr != nil {
			return perr
		}
		if perr := s.products.SetStyleTags(ctx, id, ids); perr != nil {
			return perr
		}
	}
	return err
}

func (s *Service) DeleteProduct(ctx context.Context, id, brandID uuid.UUID) error {
	err := s.products.SoftDelete(ctx, id, brandID)
	if errors.Is(err, repo.ErrNotFound) {
		return domain.ErrProductNotFound
	}
	return err
}

// ── VARIANT CRUD ──
func (s *Service) verifyProductOwned(ctx context.Context, id, brandID uuid.UUID) error {
	p, err := s.products.FindByID(ctx, id)
	if errors.Is(err, repo.ErrNotFound) || (p != nil && p.BrandID != brandID) {
		return domain.ErrProductNotFound
	}
	return err
}

func (s *Service) CreateVariant(ctx context.Context, productID, brandID uuid.UUID, req *domain.CreateVariantRequest) (*domain.Variant, error) {
	if err := s.verifyProductOwned(ctx, productID, brandID); err != nil {
		return nil, err
	}
	v, err := s.variants.Create(ctx, productID, req)
	if errors.Is(err, repo.ErrVariantConflict) {
		return nil, domain.ErrVariantConflict
	}
	return v, err
}

func (s *Service) UpdateVariant(ctx context.Context, id, productID, brandID uuid.UUID, req *domain.UpdateVariantRequest) (*domain.Variant, error) {
	if err := s.verifyProductOwned(ctx, productID, brandID); err != nil {
		return nil, err
	}
	v, err := s.variants.Update(ctx, id, productID, req)
	switch {
	case errors.Is(err, repo.ErrNotFound):
		return nil, domain.ErrVariantNotFound
	case errors.Is(err, repo.ErrVariantConflict):
		return nil, domain.ErrVariantConflict
	}
	return v, err
}

func (s *Service) DeleteVariant(ctx context.Context, id, productID, brandID uuid.UUID) error {
	if err := s.verifyProductOwned(ctx, productID, brandID); err != nil {
		return err
	}
	err := s.variants.SoftDelete(ctx, id, productID)
	if errors.Is(err, repo.ErrNotFound) {
		return domain.ErrVariantNotFound
	}
	return err
}

// ── IMAGE UPLOAD ──
func (s *Service) UploadImages(ctx context.Context, productID, brandID uuid.UUID, files []*multipart.FileHeader) ([]*domain.Image, error) {
	if err := s.verifyProductOwned(ctx, productID, brandID); err != nil {
		return nil, err
	}
	if len(files) > 10 {
		return nil, domain.ErrTooManyFiles
	}

	var created []*domain.Image
	var keysWritten []string

	rollback := func() {
		for _, k := range keysWritten {
			_ = s.storage.Delete(ctx, k)
		}
	}

	for _, fh := range files {
		if fh.Size > s.maxFileSize {
			rollback()
			return nil, domain.ErrFileTooLarge
		}
		f, err := fh.Open()
		if err != nil {
			rollback()
			return nil, domain.ErrStorageError
		}

		// Sniff first 512 bytes
		sniff := make([]byte, 512)
		n, _ := io.ReadFull(f, sniff)
		sniff = sniff[:n]
		mime := http.DetectContentType(sniff)
		ext, allowed := s.allowedMIMEs[mime]
		if !allowed {
			f.Close()
			rollback()
			return nil, domain.ErrInvalidMIME
		}

		// Reassemble reader: prepend sniffed bytes to remainder.
		body := io.MultiReader(bytes.NewReader(sniff), f)
		key := fmt.Sprintf("products/%s/%s.%s", productID.String(), uuid.New().String(), ext)
		url, err := s.storage.Put(ctx, storage.Object{Key: key, ContentType: mime, Size: fh.Size}, body)
		f.Close()
		if err != nil {
			rollback()
			return nil, domain.ErrStorageError
		}
		keysWritten = append(keysWritten, key)

		img, err := s.images.Create(ctx, productID, url, key)
		if err != nil {
			rollback()
			return nil, err
		}
		created = append(created, img)
	}
	return created, nil
}

func (s *Service) UpdateImage(ctx context.Context, id, productID, brandID uuid.UUID, req *domain.UpdateImageRequest) (*domain.Image, error) {
	if err := s.verifyProductOwned(ctx, productID, brandID); err != nil {
		return nil, err
	}
	img, err := s.images.Update(ctx, id, productID, req)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, domain.ErrImageNotFound
	}
	return img, err
}

func (s *Service) DeleteImage(ctx context.Context, id, productID, brandID uuid.UUID) error {
	if err := s.verifyProductOwned(ctx, productID, brandID); err != nil {
		return err
	}
	storageKey, wasPrimary, err := s.images.Delete(ctx, id, productID)
	if errors.Is(err, repo.ErrNotFound) {
		return domain.ErrImageNotFound
	}
	if err != nil {
		return err
	}
	if wasPrimary {
		if err := s.images.PromoteNextPrimary(ctx, productID); err != nil {
			return err
		}
	}
	_ = s.storage.Delete(ctx, storageKey) // log only; DB row already gone
	return nil
}

// ── helpers ──
func parseUUIDs(in []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(in))
	for _, s := range in {
		u, err := uuid.Parse(s)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}
