package domain

type UpdateCaptionRequest struct {
	Caption string `json:"caption" binding:"max=2000"`
}

type AddCommentRequest struct {
	Body string `json:"body" binding:"required,min=1,max=500"`
}

type ListQuery struct {
	Page  int `form:"page,default=1"   binding:"min=1"`
	Limit int `form:"limit,default=20" binding:"min=1,max=50"`
}

type ProductTagResponse struct {
	ProductID string `json:"product_id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
}

type PostResponse struct {
	ID           string               `json:"id"`
	AuthorName   string               `json:"author_name"`
	Caption      *string              `json:"caption,omitempty"`
	PhotoURLs    []string             `json:"photo_urls"`
	LikeCount    int                  `json:"like_count"`
	CommentCount int                  `json:"comment_count"`
	LikedByMe    bool                 `json:"liked_by_me"`
	Tags         []ProductTagResponse `json:"tags"`
	CreatedAt    string               `json:"created_at"`
}

type CommentResponse struct {
	ID         string `json:"id"`
	AuthorName string `json:"author_name"`
	Body       string `json:"body"`
	CreatedAt  string `json:"created_at"`
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

func ToPostResponse(v *PostView) PostResponse {
	tags := make([]ProductTagResponse, 0, len(v.Tags))
	for _, t := range v.Tags {
		tags = append(tags, ProductTagResponse{ProductID: t.ProductID.String(), Slug: t.Slug, Name: t.Name})
	}
	photos := v.PhotoURLs
	if photos == nil {
		photos = []string{}
	}
	return PostResponse{
		ID: v.ID.String(), AuthorName: v.AuthorName, Caption: v.Caption,
		PhotoURLs: photos, LikeCount: v.LikeCount, CommentCount: v.CommentCount,
		LikedByMe: v.LikedByMe, Tags: tags,
		CreatedAt: v.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func ToCommentResponse(v *CommentView) CommentResponse {
	return CommentResponse{
		ID: v.ID.String(), AuthorName: v.AuthorName, Body: v.Body,
		CreatedAt: v.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
