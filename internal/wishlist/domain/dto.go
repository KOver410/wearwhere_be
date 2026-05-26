package domain

type WishlistItemResponse struct {
	ProductID       string   `json:"product_id"`
	ProductSlug     string   `json:"product_slug"`
	ProductName     string   `json:"product_name"`
	PrimaryImageURL *string  `json:"primary_image_url,omitempty"`
	MinPrice        *float64 `json:"min_price,omitempty"`
	Brand           struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
		Name string `json:"name"`
	} `json:"brand"`
	AddedAt string `json:"added_at"`
}

type WishlistListResponse struct {
	Items      []WishlistItemResponse `json:"items"`
	Pagination struct {
		Page       int  `json:"page"`
		Limit      int  `json:"limit"`
		Total      int  `json:"total"`
		TotalPages int  `json:"total_pages"`
		HasMore    bool `json:"has_more"`
	} `json:"pagination"`
}

type WishlistContainsResponse struct {
	InWishlist map[string]bool `json:"in_wishlist"`
}

type WishlistContainsQuery struct {
	ProductIDs []string `form:"product_ids" binding:"required,min=1,max=60,dive,uuid"`
}

type WishlistListQuery struct {
	Page  int `form:"page,default=1"   binding:"min=1"`
	Limit int `form:"limit,default=24" binding:"min=1,max=60"`
}
