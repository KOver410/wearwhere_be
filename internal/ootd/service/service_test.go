package service

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"testing"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/ootd/domain"
	repo "github.com/wearwhere/wearwhere_be/internal/ootd/repo"
	"github.com/wearwhere/wearwhere_be/internal/shared/storage"
)

var repoErrNotFound = repo.ErrNotFound

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
func (f *fakeRepo) GetPost(_ context.Context, id uuid.UUID) (*domain.PostView, error) {
	if f.post == nil {
		return nil, repoErrNotFound
	}
	return f.post, nil
}
func (f *fakeRepo) FeedList(context.Context, int, int) ([]*domain.PostView, int, error) { return nil, 0, nil }
func (f *fakeRepo) ListByUser(context.Context, uuid.UUID, int, int) ([]*domain.PostView, int, error) {
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
func (f *fakeRepo) ListComments(context.Context, uuid.UUID, int, int) ([]*domain.CommentView, int, error) {
	return nil, 0, nil
}
func (f *fakeRepo) CommentOwner(_ context.Context, _ uuid.UUID) (uuid.UUID, error) { return f.owner, nil }
func (f *fakeRepo) SoftDeleteComment(context.Context, uuid.UUID) error             { return nil }

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

func newSvc(f *fakeRepo, st storage.Storage) *Service {
	return New(f, st, map[string]string{"image/jpeg": "jpg"}, 5<<20)
}

func TestCreatePost_NoPhotos_Error(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &memStorage{})
	_, err := svc.CreatePost(context.Background(), uuid.New(), "cap", nil, nil)
	if err == nil {
		t.Fatal("expected error for zero photos")
	}
}

func TestCreatePost_TooManyPhotos_Error(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &memStorage{})
	files := make([]*multipart.FileHeader, 11)
	for i := range files {
		files[i] = fileHeader(t, "p.jpg", jpegHeader)
	}
	_, err := svc.CreatePost(context.Background(), uuid.New(), "cap", files, nil)
	if err == nil {
		t.Fatal("expected error for >10 photos")
	}
}

func TestCreatePost_UploadsAndCreates(t *testing.T) {
	f := &fakeRepo{}
	st := &memStorage{}
	svc := newSvc(f, st)
	files := []*multipart.FileHeader{fileHeader(t, "p.jpg", jpegHeader)}
	post, err := svc.CreatePost(context.Background(), uuid.New(), "cap", files, nil)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if st.puts != 1 || f.created == nil || len(post.PhotoURLs) != 1 {
		t.Errorf("expected 1 upload + created post with 1 photo, got puts=%d created=%v", st.puts, f.created)
	}
}

func TestDeletePost_RejectsNonOwner(t *testing.T) {
	owner := uuid.New()
	f := &fakeRepo{post: &domain.PostView{Post: domain.Post{ID: uuid.New(), UserID: owner}}}
	svc := newSvc(f, &memStorage{})
	if err := svc.DeletePost(context.Background(), uuid.New(), f.post.ID); err == nil {
		t.Fatal("expected FORBIDDEN for non-owner")
	}
}

func TestDeleteComment_RejectsNonOwner(t *testing.T) {
	f := &fakeRepo{owner: uuid.New()}
	svc := newSvc(f, &memStorage{})
	if err := svc.DeleteComment(context.Background(), uuid.New(), uuid.New()); err == nil {
		t.Fatal("expected FORBIDDEN for non-owner comment delete")
	}
}
