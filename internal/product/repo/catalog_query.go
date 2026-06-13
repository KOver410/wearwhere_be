package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/product/domain"
)

type CatalogPG struct{ db DBTX }

func NewCatalogPG(db DBTX) *CatalogPG { return &CatalogPG{db: db} }

// List returns active products matching the query filters with pagination.
func (r *CatalogPG) List(ctx context.Context, q *domain.ListProductsQuery) ([]*domain.CatalogItem, int, error) {
	var conds []string
	var args []any
	add := func(cond string, arg any) {
		args = append(args, arg)
		conds = append(conds, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(args))))
	}

	if q.Q != "" {
		add("p.search_text % unaccent(lower(?))", q.Q)
	}
	if q.Category != "" {
		add("c.slug = ?", q.Category)
	}
	if q.Brand != "" {
		add("b.slug = ?", q.Brand)
	}
	if q.PriceMin != nil {
		add("vp.min_price >= ?", *q.PriceMin)
	}
	if q.PriceMax != nil {
		add("vp.min_price <= ?", *q.PriceMax)
	}
	if len(q.Style) > 0 {
		add(`EXISTS (
              SELECT 1 FROM product_style_tags pst
              JOIN style_tags st ON st.id = pst.style_tag_id
              WHERE pst.product_id = p.id AND st.slug = ANY(?))`, q.Style)
	}
	if len(q.Size) > 0 {
		add(`EXISTS (
              SELECT 1 FROM product_variants pv
              WHERE pv.product_id = p.id AND pv.size = ANY(?)
                AND pv.is_active AND pv.stock_qty > 0 AND pv.deleted_at IS NULL)`, q.Size)
	}
	if len(q.Color) > 0 {
		add(`EXISTS (
              SELECT 1 FROM product_variants pv
              WHERE pv.product_id = p.id AND pv.color = ANY(?)
                AND pv.is_active AND pv.stock_qty > 0 AND pv.deleted_at IS NULL)`, q.Color)
	}

	where := "p.deleted_at IS NULL AND p.status = 'active'" +
		" AND b.deleted_at IS NULL AND b.status = 'active'"
	if len(conds) > 0 {
		where += " AND " + strings.Join(conds, " AND ")
	}

	// countArgsLen: number of filter args added via add() — equals len(conds).
	// We capture this before appending sort/limit/offset args so COUNT can reuse
	// the same slice prefix without the extra positional args.
	countArgsLen := len(args)

	var orderBy string
	switch q.Sort {
	case "price_asc":
		orderBy = "vp.min_price ASC NULLS LAST, p.created_at DESC, p.id DESC"
	case "price_desc":
		orderBy = "vp.min_price DESC NULLS LAST, p.created_at DESC, p.id DESC"
	case "popular":
		orderBy = "p.sold_count DESC, p.view_count DESC, p.created_at DESC, p.id DESC"
	case "relevance":
		if q.Q != "" {
			args = append(args, q.Q)
			relIdx := len(args)
			orderBy = fmt.Sprintf(
				"similarity(p.search_text, unaccent(lower($%d))) DESC, p.sold_count DESC, p.created_at DESC, p.id DESC",
				relIdx)
		} else {
			orderBy = "p.created_at DESC, p.id DESC"
		}
	case "newest", "":
		orderBy = "p.created_at DESC, p.id DESC"
	default:
		orderBy = "p.created_at DESC, p.id DESC"
	}

	page := q.Page
	if page < 1 {
		page = 1
	}
	limit := q.Limit
	if limit < 1 || limit > 60 {
		limit = 24
	}
	offset := (page - 1) * limit

	args = append(args, limit, offset)
	limOff := fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	selectSQL := `
SELECT p.id, p.brand_id, p.category_id, p.slug, p.name, p.description, p.status::text,
       p.currency, p.sold_count, p.view_count, p.created_at, p.updated_at, p.deleted_at,
       b.slug AS brand_slug, b.name AS brand_name,
       vp.min_price, vp.max_price, vp.in_stock,
       (SELECT url FROM product_images
         WHERE product_id = p.id ORDER BY sort_order ASC LIMIT 1) AS primary_image
  FROM products p
  JOIN brands b      ON b.id = p.brand_id
  JOIN categories c  ON c.id = p.category_id
  JOIN LATERAL (
    SELECT MIN(price) AS min_price, MAX(price) AS max_price,
           bool_or(stock_qty > 0) AS in_stock
      FROM product_variants
     WHERE product_id = p.id AND deleted_at IS NULL AND is_active
  ) vp ON true
 WHERE ` + where + `
 ORDER BY ` + orderBy + limOff

	rows, err := r.db.Query(ctx, selectSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []*domain.CatalogItem
	for rows.Next() {
		var it domain.CatalogItem
		var status string
		var minP, maxP *float64
		var inStock *bool
		var primary *string
		if err := rows.Scan(
			&it.ID, &it.BrandID, &it.CategoryID, &it.Slug, &it.Name, &it.Description,
			&status, &it.Currency, &it.SoldCount, &it.ViewCount,
			&it.CreatedAt, &it.UpdatedAt, &it.DeletedAt,
			&it.BrandSlug, &it.BrandName,
			&minP, &maxP, &inStock, &primary,
		); err != nil {
			return nil, 0, err
		}
		it.Status = domain.ProductStatus(status)
		if minP != nil {
			it.MinPrice = *minP
		}
		if maxP != nil {
			it.MaxPrice = *maxP
		}
		if inStock != nil {
			it.InStock = *inStock
		}
		it.PrimaryImage = primary
		items = append(items, &it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	// Count query — same WHERE, no ORDER/LIMIT.
	// Use only the filter args (not the relevance arg or limit/offset).
	countSQL := `
SELECT COUNT(*)
  FROM products p
  JOIN brands b      ON b.id = p.brand_id
  JOIN categories c  ON c.id = p.category_id
  JOIN LATERAL (
    SELECT MIN(price) AS min_price
      FROM product_variants
     WHERE product_id = p.id AND deleted_at IS NULL AND is_active
  ) vp ON true
 WHERE ` + where

	countArgs := args[:countArgsLen]
	var total int
	if err := r.db.QueryRow(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// productColsP is productCols with "p." table prefix for queries that join another table.
const productColsP = `p.id, p.brand_id, p.category_id, p.slug, p.name, p.description, p.status,
                      p.currency, p.sold_count, p.view_count, p.created_at, p.updated_at, p.deleted_at`

// productColsPDetail extends productColsP with the denormalized rating columns.
const productColsPDetail = productColsP + `, p.avg_rating, p.review_count`

// scanProductDetail scans a product row that includes avg_rating and review_count.
func scanProductDetail(row pgx.Row) (*domain.Product, error) {
	var p domain.Product
	var status string
	err := row.Scan(
		&p.ID, &p.BrandID, &p.CategoryID, &p.Slug, &p.Name, &p.Description, &status,
		&p.Currency, &p.SoldCount, &p.ViewCount, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
		&p.AvgRating, &p.ReviewCount,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.Status = domain.ProductStatus(status)
	return &p, nil
}

// Detail returns product + category + variants + images + style tags by brand and product slug.
func (r *CatalogPG) Detail(ctx context.Context, brandSlug, productSlug string) (
	*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error,
) {
	prodRow := r.db.QueryRow(ctx,
		`SELECT `+productColsPDetail+` FROM products p
         JOIN brands b ON b.id = p.brand_id
         WHERE b.slug=$1 AND p.slug=$2
           AND p.deleted_at IS NULL AND b.deleted_at IS NULL`,
		brandSlug, productSlug)
	p, err := scanProductDetail(prodRow)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, nil, nil, nil, err
		}
		return nil, nil, nil, nil, nil, err
	}
	return r.collectDetailParts(ctx, p)
}

// DetailByID returns product + category + variants + images + style tags by product UUID.
func (r *CatalogPG) DetailByID(ctx context.Context, id uuid.UUID) (
	*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error,
) {
	p, err := scanProduct(r.db.QueryRow(ctx,
		`SELECT `+productCols+` FROM products WHERE id=$1 AND deleted_at IS NULL`, id))
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	return r.collectDetailParts(ctx, p)
}

func (r *CatalogPG) collectDetailParts(ctx context.Context, p *domain.Product) (
	*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error,
) {
	var cat domain.Category
	if err := r.db.QueryRow(ctx,
		`SELECT id, slug, name, display_order FROM categories WHERE id=$1`, p.CategoryID).
		Scan(&cat.ID, &cat.Slug, &cat.Name, &cat.DisplayOrder); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	variantRows, err := r.db.Query(ctx,
		`SELECT `+variantCols+` FROM product_variants
         WHERE product_id=$1 AND deleted_at IS NULL ORDER BY created_at ASC`, p.ID)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	var variants []*domain.Variant
	for variantRows.Next() {
		v, err := scanVariant(variantRows)
		if err != nil {
			variantRows.Close()
			return nil, nil, nil, nil, nil, err
		}
		variants = append(variants, v)
	}
	variantRows.Close()
	if err := variantRows.Err(); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	imageRows, err := r.db.Query(ctx,
		`SELECT `+imageCols+` FROM product_images
         WHERE product_id=$1 ORDER BY sort_order ASC`, p.ID)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	var images []*domain.Image
	for imageRows.Next() {
		i, err := scanImage(imageRows)
		if err != nil {
			imageRows.Close()
			return nil, nil, nil, nil, nil, err
		}
		images = append(images, i)
	}
	imageRows.Close()
	if err := imageRows.Err(); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	tagRows, err := r.db.Query(ctx,
		`SELECT s.id, s.slug, s.name
           FROM style_tags s
           JOIN product_style_tags pst ON pst.style_tag_id = s.id
          WHERE pst.product_id = $1 ORDER BY s.name`, p.ID)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	var tags []*domain.StyleTag
	for tagRows.Next() {
		var st domain.StyleTag
		if err := tagRows.Scan(&st.ID, &st.Slug, &st.Name); err != nil {
			tagRows.Close()
			return nil, nil, nil, nil, nil, err
		}
		tags = append(tags, &st)
	}
	tagRows.Close()
	if err := tagRows.Err(); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	return p, &cat, variants, images, tags, nil
}

// Suggestions returns up to limit product names matching the search query via trigram similarity.
func (r *CatalogPG) Suggestions(ctx context.Context, q string, limit int) ([]string, error) {
	if q == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 3
	}
	rows, err := r.db.Query(ctx,
		`SELECT name
           FROM products
          WHERE status = 'active' AND deleted_at IS NULL
          ORDER BY similarity(unaccent(lower(name)), unaccent(lower($1))) DESC
          LIMIT $2`, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// compile-time interface check
var _ CatalogRepo = (*CatalogPG)(nil)
