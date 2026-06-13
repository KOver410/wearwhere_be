package domain

// WriteReviewRequest is the body for create (POST) and update (PATCH).
// Gin binding enforces the SRS rules: rating 1-5, body >= 20 chars,
// fit (optional) one of small|true|large.
type WriteReviewRequest struct {
	Rating int    `json:"rating" binding:"required,min=1,max=5"`
	Body   string `json:"body"   binding:"required,min=20,max=5000"`
	Fit    string `json:"fit"    binding:"omitempty,oneof=small true large"`
}

// ListReviewsQuery is the query string for GET /products/:id/reviews.
type ListReviewsQuery struct {
	Rating int    `form:"rating" binding:"omitempty,min=1,max=5"`
	Fit    string `form:"fit"    binding:"omitempty,oneof=small true large"`
	Sort   string `form:"sort"   binding:"omitempty,oneof=newest rating_high rating_low"`
	Page   int    `form:"page,default=1"   binding:"min=1"`
	Limit  int    `form:"limit,default=20" binding:"min=1,max=50"`
}

type ReviewResponse struct {
	ID           string  `json:"id"`
	Rating       int     `json:"rating"`
	Body         string  `json:"body"`
	Fit          *string `json:"fit,omitempty"`
	Verified     bool    `json:"verified"` // always true
	ReviewerName string  `json:"reviewer_name"`
	CreatedAt    string  `json:"created_at"`
}

type ListReviewsResponse struct {
	Items       []ReviewResponse `json:"items"`
	AvgRating   float64          `json:"avg_rating"`
	ReviewCount int              `json:"review_count"`
	Pagination  Pagination       `json:"pagination"`
}

type Pagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

func NewPagination(page, limit, total int) Pagination {
	tp := 0
	if limit > 0 {
		tp = (total + limit - 1) / limit
	}
	return Pagination{Page: page, Limit: limit, Total: total, TotalPages: tp}
}

func ToReviewResponse(v *ReviewView) ReviewResponse {
	return ReviewResponse{
		ID:           v.ID.String(),
		Rating:       v.Rating,
		Body:         v.Body,
		Fit:          v.Fit,
		Verified:     true,
		ReviewerName: v.ReviewerName,
		CreatedAt:    v.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
