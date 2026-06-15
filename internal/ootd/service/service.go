// Package service holds OOTD business logic: photo upload, validation,
// ownership, and like/comment orchestration.
package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/ootd/domain"
	"github.com/wearwhere/wearwhere_be/internal/ootd/repo"
	"github.com/wearwhere/wearwhere_be/internal/shared/storage"
)

type Service struct {
	repo         repo.Repo
	storage      storage.Storage
	allowedMIMEs map[string]string // mime -> ext
	maxFileSize  int64
}

func New(r repo.Repo, st storage.Storage, allowedMIMEs map[string]string, maxFileSize int64) *Service {
	return &Service{repo: r, storage: st, allowedMIMEs: allowedMIMEs, maxFileSize: maxFileSize}
}

// CreatePost uploads photos then creates the post + tags. caption "" → nil.
func (s *Service) CreatePost(ctx context.Context, userID uuid.UUID, caption string, files []*multipart.FileHeader, productIDs []uuid.UUID) (*domain.Post, error) {
	if len([]rune(caption)) > 2000 {
		return nil, domain.ErrCaptionTooLong()
	}
	if len(files) == 0 {
		return nil, domain.ErrNoPhotos()
	}
	if len(files) > 10 {
		return nil, domain.ErrTooManyPhotos()
	}
	postID := uuid.New()
	urls, keys, err := s.uploadPhotos(ctx, postID, files)
	if err != nil {
		return nil, err
	}
	var capPtr *string
	if caption != "" {
		capPtr = &caption
	}
	post := &domain.Post{ID: postID, UserID: userID, Caption: capPtr, PhotoURLs: urls}
	if err := s.repo.CreatePost(ctx, post, productIDs); err != nil {
		for _, k := range keys {
			_ = s.storage.Delete(ctx, k)
		}
		return nil, err
	}
	return post, nil
}

func (s *Service) uploadPhotos(ctx context.Context, postID uuid.UUID, files []*multipart.FileHeader) ([]string, []string, error) {
	var urls, keys []string
	rollback := func() {
		for _, k := range keys {
			_ = s.storage.Delete(ctx, k)
		}
	}
	for _, fh := range files {
		if fh.Size > s.maxFileSize {
			rollback()
			return nil, nil, domain.ErrFileTooLarge()
		}
		f, err := fh.Open()
		if err != nil {
			rollback()
			return nil, nil, domain.ErrStorageError()
		}
		sniff := make([]byte, 512)
		n, _ := io.ReadFull(f, sniff)
		sniff = sniff[:n]
		mime := http.DetectContentType(sniff)
		ext, ok := s.allowedMIMEs[mime]
		if !ok {
			f.Close()
			rollback()
			return nil, nil, domain.ErrInvalidMIME()
		}
		body := io.MultiReader(bytes.NewReader(sniff), f)
		key := fmt.Sprintf("ootd/%s/%s.%s", postID.String(), uuid.New().String(), ext)
		url, err := s.storage.Put(ctx, storage.Object{Key: key, ContentType: mime, Size: fh.Size}, body)
		f.Close()
		if err != nil {
			rollback()
			return nil, nil, domain.ErrStorageError()
		}
		urls = append(urls, url)
		keys = append(keys, key)
	}
	return urls, keys, nil
}

// enrich fills Tags + LikedByMe for a set of post views. viewerID == uuid.Nil → guest.
func (s *Service) enrich(ctx context.Context, views []*domain.PostView, viewerID uuid.UUID) error {
	if len(views) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(views))
	for i, v := range views {
		ids[i] = v.ID
	}
	tags, err := s.repo.TagsForPosts(ctx, ids)
	if err != nil {
		return err
	}
	liked := map[uuid.UUID]bool{}
	if viewerID != uuid.Nil {
		liked, err = s.repo.LikedPostIDs(ctx, viewerID, ids)
		if err != nil {
			return err
		}
	}
	for _, v := range views {
		v.Tags = tags[v.ID]
		v.LikedByMe = liked[v.ID]
	}
	return nil
}

func (s *Service) Following(ctx context.Context, viewerID uuid.UUID, page, limit int) ([]*domain.PostView, int, error) {
	views, total, err := s.repo.FollowedFeed(ctx, viewerID, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, err
	}
	if err := s.enrich(ctx, views, viewerID); err != nil {
		return nil, 0, err
	}
	return views, total, nil
}

func (s *Service) Feed(ctx context.Context, viewerID uuid.UUID, page, limit int) ([]*domain.PostView, int, error) {
	views, total, err := s.repo.FeedList(ctx, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, err
	}
	if err := s.enrich(ctx, views, viewerID); err != nil {
		return nil, 0, err
	}
	return views, total, nil
}

func (s *Service) ByUser(ctx context.Context, viewerID, userID uuid.UUID, page, limit int) ([]*domain.PostView, int, error) {
	views, total, err := s.repo.ListByUser(ctx, userID, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, err
	}
	if err := s.enrich(ctx, views, viewerID); err != nil {
		return nil, 0, err
	}
	return views, total, nil
}

func (s *Service) GetPost(ctx context.Context, viewerID, postID uuid.UUID) (*domain.PostView, error) {
	v, err := s.repo.GetPost(ctx, postID)
	if err != nil {
		return nil, domain.ErrPostNotFound()
	}
	if err := s.enrich(ctx, []*domain.PostView{v}, viewerID); err != nil {
		return nil, err
	}
	return v, nil
}

func (s *Service) UpdateCaption(ctx context.Context, userID, postID uuid.UUID, caption string) error {
	if len([]rune(caption)) > 2000 {
		return domain.ErrCaptionTooLong()
	}
	v, err := s.repo.GetPost(ctx, postID)
	if err != nil {
		return domain.ErrPostNotFound()
	}
	if v.UserID != userID {
		return domain.ErrForbidden()
	}
	var capPtr *string
	if caption != "" {
		capPtr = &caption
	}
	if err := s.repo.UpdateCaption(ctx, postID, capPtr); err != nil {
		return domain.ErrPostNotFound()
	}
	return nil
}

func (s *Service) DeletePost(ctx context.Context, userID, postID uuid.UUID) error {
	v, err := s.repo.GetPost(ctx, postID)
	if err != nil {
		return domain.ErrPostNotFound()
	}
	if v.UserID != userID {
		return domain.ErrForbidden()
	}
	if err := s.repo.SoftDeletePost(ctx, postID); err != nil {
		return domain.ErrPostNotFound()
	}
	return nil
}

func (s *Service) Like(ctx context.Context, userID, postID uuid.UUID) error {
	if _, err := s.repo.GetPost(ctx, postID); err != nil {
		return domain.ErrPostNotFound()
	}
	return s.repo.Like(ctx, postID, userID)
}

func (s *Service) Unlike(ctx context.Context, userID, postID uuid.UUID) error {
	return s.repo.Unlike(ctx, postID, userID)
}

func (s *Service) AddComment(ctx context.Context, userID, postID uuid.UUID, body string) (*domain.Comment, error) {
	if _, err := s.repo.GetPost(ctx, postID); err != nil {
		return nil, domain.ErrPostNotFound()
	}
	c := &domain.Comment{PostID: postID, UserID: userID, Body: body}
	if err := s.repo.AddComment(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) ListComments(ctx context.Context, postID uuid.UUID, page, limit int) ([]*domain.CommentView, int, error) {
	return s.repo.ListComments(ctx, postID, limit, (page-1)*limit)
}

func (s *Service) DeleteComment(ctx context.Context, userID, commentID uuid.UUID) error {
	owner, err := s.repo.CommentOwner(ctx, commentID)
	if err != nil {
		return domain.ErrCommentNotFound()
	}
	if owner != userID {
		return domain.ErrForbidden()
	}
	if err := s.repo.SoftDeleteComment(ctx, commentID); err != nil {
		return domain.ErrCommentNotFound()
	}
	return nil
}
