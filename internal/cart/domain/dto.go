package domain

type AddToCartRequest struct {
	VariantID string `json:"variant_id" binding:"required,uuid"`
	Qty       int    `json:"qty"        binding:"required,min=1,max=10"`
}

type UpdateCartItemRequest struct {
	Qty int `json:"qty" binding:"required,min=1,max=10"`
}

type CartItemResponse struct {
	ID               string  `json:"id"`
	Qty              int     `json:"qty"`
	PriceSnapshot    string  `json:"price_snapshot"`
	CurrentPrice     string  `json:"current_price"`
	PriceChanged     bool    `json:"price_changed"`
	SubtotalSnapshot string  `json:"subtotal_snapshot"`
	SubtotalCurrent  string  `json:"subtotal_current"`
	Currency         string  `json:"currency"`
	Unavailable      bool    `json:"unavailable"`
	UnavailableReason *string `json:"unavailable_reason,omitempty"`
	AddedAt          string  `json:"added_at"`

	Variant struct {
		ID       string  `json:"id"`
		SKU      string  `json:"sku"`
		Size     string  `json:"size"`
		Color    string  `json:"color"`
		ColorHex *string `json:"color_hex,omitempty"`
		StockQty int     `json:"stock_qty"`
	} `json:"variant"`

	Product struct {
		ID              string  `json:"id"`
		Slug            string  `json:"slug"`
		Name            string  `json:"name"`
		PrimaryImageURL *string `json:"primary_image_url,omitempty"`
	} `json:"product"`

	Brand struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
		Name string `json:"name"`
	} `json:"brand"`
}

type CartSummary struct {
	ItemCount       int    `json:"item_count"`
	TotalQty        int    `json:"total_qty"`
	TotalSnapshot   string `json:"total_snapshot"`
	TotalCurrent    string `json:"total_current"`
	Currency        string `json:"currency"`
	HasPriceChanges bool   `json:"has_price_changes"`
	HasUnavailable  bool   `json:"has_unavailable"`
}

type CartResponse struct {
	Items   []CartItemResponse `json:"items"`
	Summary CartSummary        `json:"summary"`
}
