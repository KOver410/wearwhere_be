package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/ootd/domain"
	"github.com/wearwhere/wearwhere_be/internal/ootd/service"
	"github.com/wearwhere/wearwhere_be/internal/shared/storage"
)

// fakeRepo is a minimal in-memory repo.Repo for handler tests.
type fakeRepo struct {
	created   *domain.Post
	createErr error
	owner     uuid.UUID
	post      *domain.PostView
}

func (f *fakeRepo) CreatePost(_ context.Context, p *domain.Post, _ []uuid.UUID) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = p
	return nil
}
func (f *fakeRepo) GetPost(_ context.Context, _ uuid.UUID) (*domain.PostView, error) {
	if f.post == nil {
		return nil, domain.ErrPostNotFound()
	}
	return f.post, nil
}
func (f *fakeRepo) FeedList(context.Context, uuid.UUID, int, int) ([]*domain.PostView, int, error) {
	return nil, 0, nil
}
func (f *fakeRepo) ListByUser(context.Context, uuid.UUID, uuid.UUID, int, int) ([]*domain.PostView, int, error) {
	return nil, 0, nil
}
func (f *fakeRepo) UpdateCaption(context.Context, uuid.UUID, *string) error { return nil }
func (f *fakeRepo) SoftDeletePost(context.Context, uuid.UUID) error         { return nil }
func (f *fakeRepo) Like(context.Context, uuid.UUID, uuid.UUID) error        { return nil }
func (f *fakeRepo) Unlike(context.Context, uuid.UUID, uuid.UUID) error      { return nil }
func (f *fakeRepo) LikedPostIDs(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID]bool, error) {
	return map[uuid.UUID]bool{}, nil
}
func (f *fakeRepo) TagsForPosts(context.Context, []uuid.UUID) (map[uuid.UUID][]domain.ProductTag, error) {
	return map[uuid.UUID][]domain.ProductTag{}, nil
}
func (f *fakeRepo) AddComment(_ context.Context, c *domain.Comment) error { c.ID = uuid.New(); return nil }
func (f *fakeRepo) ListComments(context.Context, uuid.UUID, uuid.UUID, int, int) ([]*domain.CommentView, int, error) {
	return nil, 0, nil
}
func (f *fakeRepo) CommentOwner(_ context.Context, _ uuid.UUID) (uuid.UUID, error) { return f.owner, nil }
func (f *fakeRepo) SoftDeleteComment(context.Context, uuid.UUID) error             { return nil }
func (f *fakeRepo) FollowedFeed(context.Context, uuid.UUID, int, int) ([]*domain.PostView, int, error) {
	return nil, 0, nil
}
func (f *fakeRepo) IsBlocked(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, nil
}

// memStorage is an in-memory storage.Storage for tests.
type memStorage struct{ puts int }

func (m *memStorage) Put(_ context.Context, obj storage.Object, _ io.Reader) (string, error) {
	m.puts++
	return "http://test/" + obj.Key, nil
}
func (m *memStorage) Delete(context.Context, string) error { return nil }
func (m *memStorage) URL(key string) string                { return "http://test/" + key }

// jpegHeader is a minimal valid JPEG magic so http.DetectContentType returns image/jpeg.
var jpegHeader = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}

func fileHeader(t *testing.T, name string, body []byte) *multipart.FileHeader {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("photos", name)
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(body)
	w.Close()
	mr := multipart.NewReader(&buf, w.Boundary())
	form, err := mr.ReadForm(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	return form.File["photos"][0]
}

// postView returns a *domain.PostView with a random ID for use in tests that need GetPost to succeed.
func postView() *domain.PostView {
	return &domain.PostView{
		Post: domain.Post{
			ID:        uuid.New(),
			UserID:    uuid.New(),
			PhotoURLs: []string{"http://test/photo.jpg"},
		},
	}
}

func setup(svc *service.Service, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := New(svc)
	v1 := r.Group("/api/v1")
	MountOOTDPublic(v1, h)
	authed := v1.Group("", func(c *gin.Context) { authmw.SetUserIDForTest(c, userID); c.Next() })
	MountOOTDAuthed(authed, h)
	return r
}

func TestCreate_NoPhotos_400(t *testing.T) {
	svc := service.New(&fakeRepo{}, &memStorage{}, map[string]string{"image/jpeg": "jpg"}, 5<<20)
	r := setup(svc, uuid.New())
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.WriteField("caption", "no photos here")
	w.Close()
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/ootd", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAddComment_InvalidBody_400(t *testing.T) {
	svc := service.New(&fakeRepo{post: postView()}, &memStorage{}, map[string]string{"image/jpeg": "jpg"}, 5<<20)
	r := setup(svc, uuid.New())
	rec := httptest.NewRecorder()
	payload, _ := json.Marshal(map[string]any{"body": ""})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/ootd/"+uuid.New().String()+"/comments", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}
