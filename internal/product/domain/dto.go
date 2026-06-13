package domain

import "time"

type CreateProductRequest struct {
	Name        string   `json:"name"          binding:"required,min=2,max=200"`
	Slug        string   `json:"slug"          binding:"omitempty,slug,max=200"`
	Description string   `json:"description"   binding:"omitempty,max=5000"`
	CategoryID  string   `json:"category_id"   binding:"required,uuid"`
	StyleTagIDs []string `json:"style_tag_ids" binding:"omitempty,max=10,dive,uuid"`
}

type UpdateProductRequest struct {
	Name        *string  `json:"name"          binding:"omitempty,min=2,max=200"`
	Slug        *string  `json:"slug"          binding:"omitempty,slug,max=200"`
	Description *string  `json:"description"   binding:"omitempty,max=5000"`
	CategoryID  *string  `json:"category_id"   binding:"omitempty,uuid"`
	Status      *string  `json:"status"        binding:"omitempty,oneof=draft active archived"`
	StyleTagIDs []string `json:"style_tag_ids" binding:"omitempty,max=10,dive,uuid"`
}

type CreateVariantRequest struct {
	SKU      string  `json:"sku"       binding:"required,min=1,max=64"`
	Size     string  `json:"size"      binding:"required,max=20"`
	Color    string  `json:"color"     binding:"required,max=50"`
	ColorHex string  `json:"color_hex" binding:"omitempty,hexcolor"`
	Price    float64 `json:"price"     binding:"required,gt=0"`
	StockQty int     `json:"stock_qty" binding:"min=0"`
	ImageID  string  `json:"image_id"  binding:"omitempty,uuid"`
}

type UpdateVariantRequest struct {
	Size     *string  `json:"size"      binding:"omitempty,max=20"`
	Color    *string  `json:"color"     binding:"omitempty,max=50"`
	ColorHex *string  `json:"color_hex" binding:"omitempty,hexcolor"`
	Price    *float64 `json:"price"     binding:"omitempty,gt=0"`
	StockQty *int     `json:"stock_qty" binding:"omitempty,min=0"`
	IsActive *bool    `json:"is_active"`
	ImageID  *string  `json:"image_id"  binding:"omitempty,uuid"`
}

type UpdateImageRequest struct {
	SortOrder *int    `json:"sort_order" binding:"omitempty,min=0"`
	AltText   *string `json:"alt_text"   binding:"omitempty,max=200"`
	IsPrimary *bool   `json:"is_primary"`
}

type ListProductsQuery struct {
	Q        string   `form:"q"          binding:"omitempty,max=100"`
	Category string   `form:"category"   binding:"omitempty,slug"`
	Brand    string   `form:"brand"      binding:"omitempty,slug"`
	Style    []string `form:"style"      binding:"omitempty,max=10,dive,slug"`
	Size     []string `form:"size"       binding:"omitempty,max=10,dive,max=20"`
	Color    []string `form:"color"      binding:"omitempty,max=10,dive,max=50"`
	PriceMin *float64 `form:"price_min"  binding:"omitempty,gte=0"`
	PriceMax *float64 `form:"price_max"  binding:"omitempty,gtefield=PriceMin"`
	Sort     string   `form:"sort"       binding:"omitempty,oneof=relevance newest popular price_asc price_desc"`
	Page     int      `form:"page,default=1"   binding:"min=1"`
	Limit    int      `form:"limit,default=24" binding:"min=1,max=60"`
}

// ── responses ──
type ProductSummary struct {
	ID           string  `json:"id"`
	Slug         string  `json:"slug"`
	Name         string  `json:"name"`
	BrandSlug    string  `json:"brand_slug"`
	BrandName    string  `json:"brand_name"`
	Currency     string  `json:"currency"`
	MinPrice     float64 `json:"min_price"`
	MaxPrice     float64 `json:"max_price"`
	InStock      bool    `json:"in_stock"`
	PrimaryImage *string `json:"primary_image,omitempty"`
}

type ProductDetail struct {
	ID          string        `json:"id"`
	Slug        string        `json:"slug"`
	Name        string        `json:"name"`
	Description *string       `json:"description,omitempty"`
	Status      string        `json:"status"`
	Currency    string        `json:"currency"`
	Brand       *BrandRef     `json:"brand"`
	Category    *CategoryRef  `json:"category"`
	StyleTags   []StyleTagRef `json:"style_tags"`
	Variants    []VariantResp `json:"variants"`
	Images      []ImageResp   `json:"images"`
	AvgRating   float64       `json:"avg_rating"`
	ReviewCount int           `json:"review_count"`
	CreatedAt   string        `json:"created_at"`
}

type BrandRef struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type CategoryRef struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type StyleTagRef struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type VariantResp struct {
	ID       string  `json:"id"`
	SKU      string  `json:"sku"`
	Size     string  `json:"size"`
	Color    string  `json:"color"`
	ColorHex *string `json:"color_hex,omitempty"`
	Price    float64 `json:"price"`
	StockQty int     `json:"stock_qty"`
	IsActive bool    `json:"is_active"`
	ImageID  *string `json:"image_id,omitempty"`
}

type ImageResp struct {
	ID        string  `json:"id"`
	URL       string  `json:"url"`
	AltText   *string `json:"alt_text,omitempty"`
	SortOrder int     `json:"sort_order"`
	IsPrimary bool    `json:"is_primary"`
}

func ToVariantResp(v *Variant) VariantResp {
	var img *string
	if v.ImageID != nil {
		s := v.ImageID.String()
		img = &s
	}
	return VariantResp{
		ID: v.ID.String(), SKU: v.SKU, Size: v.Size, Color: v.Color,
		ColorHex: v.ColorHex, Price: v.Price, StockQty: v.StockQty,
		IsActive: v.IsActive, ImageID: img,
	}
}

func ToImageResp(i *Image) ImageResp {
	return ImageResp{
		ID: i.ID.String(), URL: i.URL, AltText: i.AltText,
		SortOrder: i.SortOrder, IsPrimary: i.IsPrimary,
	}
}

func ToProductSummary(c *CatalogItem) ProductSummary {
	return ProductSummary{
		ID: c.ID.String(), Slug: c.Slug, Name: c.Name,
		BrandSlug: c.BrandSlug, BrandName: c.BrandName,
		Currency: c.Currency,
		MinPrice: c.MinPrice, MaxPrice: c.MaxPrice, InStock: c.InStock,
		PrimaryImage: c.PrimaryImage,
	}
}

// Format helper for time fields.
func FormatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }
