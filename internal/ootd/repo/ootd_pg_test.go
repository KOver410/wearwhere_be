//go:build integration

package repo

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/ootd/domain"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		panic("TEST_DATABASE_URL not set; run via `make test-integration`")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		panic(err)
	}
	testPool = pool
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func ptr(s string) *string { return &s }

// makePost creates a post owned by a fresh customer; returns post id + user id.
func makePost(t *testing.T, r *OOTDPg, productIDs []uuid.UUID) (uuid.UUID, uuid.UUID) {
	t.Helper()
	u := testfixtures.SeedCustomer(t, testPool)
	p := &domain.Post{ID: uuid.New(), UserID: u.ID, Caption: ptr("my outfit"), PhotoURLs: []string{"http://x/1.jpg"}}
	require.NoError(t, r.CreatePost(context.Background(), p, productIDs))
	return p.ID, u.ID
}

func TestOOTD_CreateAndGet_WithTags(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewOOTDPg(testPool)
	sb := testfixtures.SeedBrand(t, testPool, uuid.Nil)
	cat := testfixtures.SeedCategory(t, testPool)
	prod := testfixtures.SeedProduct(t, testPool, sb.ID, cat.ID, "active")

	postID, _ := makePost(t, r, []uuid.UUID{prod.ID})
	got, err := r.GetPost(ctx, postID)
	require.NoError(t, err)
	require.Equal(t, "my outfit", *got.Caption)
	require.Len(t, got.PhotoURLs, 1)

	tags, err := r.TagsForPosts(ctx, []uuid.UUID{postID})
	require.NoError(t, err)
	require.Len(t, tags[postID], 1)
	require.Equal(t, prod.ID, tags[postID][0].ProductID)
}

func TestOOTD_Like_Idempotent(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewOOTDPg(testPool)
	postID, _ := makePost(t, r, nil)
	liker := testfixtures.SeedCustomer(t, testPool)

	require.NoError(t, r.Like(ctx, postID, liker.ID))
	require.NoError(t, r.Like(ctx, postID, liker.ID)) // idempotent
	got, _ := r.GetPost(ctx, postID)
	require.Equal(t, 1, got.LikeCount)

	liked, err := r.LikedPostIDs(ctx, liker.ID, []uuid.UUID{postID})
	require.NoError(t, err)
	require.True(t, liked[postID])

	require.NoError(t, r.Unlike(ctx, postID, liker.ID))
	require.NoError(t, r.Unlike(ctx, postID, liker.ID)) // idempotent
	got, _ = r.GetPost(ctx, postID)
	require.Equal(t, 0, got.LikeCount)
}

func TestOOTD_Comment_AddListDelete_Counts(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewOOTDPg(testPool)
	postID, _ := makePost(t, r, nil)
	commenter := testfixtures.SeedCustomer(t, testPool)

	c := &domain.Comment{PostID: postID, UserID: commenter.ID, Body: "nice fit!"}
	require.NoError(t, r.AddComment(ctx, c))
	got, _ := r.GetPost(ctx, postID)
	require.Equal(t, 1, got.CommentCount)

	list, total, err := r.ListComments(ctx, postID, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, list, 1)
	require.Equal(t, "nice fit!", list[0].Body)
	require.NotEmpty(t, list[0].AuthorName)

	owner, err := r.CommentOwner(ctx, c.ID)
	require.NoError(t, err)
	require.Equal(t, commenter.ID, owner)

	require.NoError(t, r.SoftDeleteComment(ctx, c.ID))
	got, _ = r.GetPost(ctx, postID)
	require.Equal(t, 0, got.CommentCount)
	require.ErrorIs(t, r.SoftDeleteComment(ctx, c.ID), ErrNotFound)
}

func TestOOTD_FeedAndByUser_ExcludeDeleted(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewOOTDPg(testPool)
	p1, u1 := makePost(t, r, nil)
	p2, _ := makePost(t, r, nil)
	require.NoError(t, r.SoftDeletePost(ctx, p2))

	feed, total, err := r.FeedList(ctx, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, feed, 1)
	require.Equal(t, p1, feed[0].ID)

	byUser, total, err := r.ListByUser(ctx, u1, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, p1, byUser[0].ID)

	// deleted post not gettable
	_, err = r.GetPost(ctx, p2)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestOOTD_UpdateCaption(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewOOTDPg(testPool)
	postID, _ := makePost(t, r, nil)
	require.NoError(t, r.UpdateCaption(ctx, postID, ptr("updated caption")))
	got, _ := r.GetPost(ctx, postID)
	require.Equal(t, "updated caption", *got.Caption)
	require.ErrorIs(t, r.UpdateCaption(ctx, uuid.New(), ptr("x")), ErrNotFound)
}

func TestOOTD_FollowedFeed(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	r := NewOOTDPg(testPool)
	viewer := testfixtures.SeedCustomer(t, testPool)
	// author the viewer follows
	postID, authorID := makePost(t, r, nil)
	// a post by someone the viewer does NOT follow
	makePost(t, r, nil)
	// viewer follows authorID
	_, err := testPool.Exec(ctx, `INSERT INTO user_follows (follower_id, followee_id) VALUES ($1,$2)`, viewer.ID, authorID)
	require.NoError(t, err)

	feed, total, err := r.FollowedFeed(ctx, viewer.ID, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, feed, 1)
	require.Equal(t, postID, feed[0].ID)
}
