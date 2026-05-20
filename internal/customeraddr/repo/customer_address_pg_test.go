//go:build integration

package repo_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
	"github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		panic("TEST_DATABASE_URL required for integration tests")
	}
	var err error
	pool, err = pgxpool.New(context.Background(), dsn)
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	os.Exit(m.Run())
}

func TestAddressPG_Create_FirstAddressIsDefault(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	r := repo.NewAddressPG(tx)

	addr, err := r.Create(context.Background(), user.ID, &domain.CreateAddressRequest{
		Label: "Nhà", RecipientName: "X", RecipientPhone: "+84901234567",
		AddressLine: "1 A", Ward: "Phường 1", District: "Quận 1", City: "TP HCM",
		IsDefault: false,
	})
	require.NoError(t, err)
	require.True(t, addr.IsDefault)
}

func TestAddressPG_Create_DefaultSwapsExisting(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	first := testfixtures.SeedCustomerAddress(t, tx, user.ID, testfixtures.CustomerAddressOpts{IsDefault: true})
	r := repo.NewAddressPG(tx)

	second, err := r.Create(context.Background(), user.ID, &domain.CreateAddressRequest{
		Label: "Office", RecipientName: "X", RecipientPhone: "+84901234567",
		AddressLine: "2 B", Ward: "P 2", District: "Q 2", City: "TP HCM",
		IsDefault: true,
	})
	require.NoError(t, err)
	require.True(t, second.IsDefault)

	refetchedFirst, err := r.FindByID(context.Background(), first.ID, user.ID)
	require.NoError(t, err)
	require.False(t, refetchedFirst.IsDefault)
}

func TestAddressPG_SoftDelete_PromotesOldestRemaining(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	older := testfixtures.SeedCustomerAddress(t, tx, user.ID, testfixtures.CustomerAddressOpts{IsDefault: false})
	defaultAddr := testfixtures.SeedCustomerAddress(t, tx, user.ID, testfixtures.CustomerAddressOpts{IsDefault: true})
	r := repo.NewAddressPG(tx)

	require.NoError(t, r.SoftDelete(context.Background(), defaultAddr.ID, user.ID))

	promoted, err := r.FindByID(context.Background(), older.ID, user.ID)
	require.NoError(t, err)
	require.True(t, promoted.IsDefault)
}

func TestAddressPG_IDOR_OtherUserCannotFind(t *testing.T) {
	tx := testfixtures.BeginTx(t, pool)
	owner := testfixtures.SeedCustomer(t, tx)
	intruder := testfixtures.SeedCustomer(t, tx)
	seeded := testfixtures.SeedCustomerAddress(t, tx, owner.ID, testfixtures.CustomerAddressOpts{})
	r := repo.NewAddressPG(tx)

	_, err := r.FindByID(context.Background(), seeded.ID, intruder.ID)
	require.ErrorIs(t, err, repo.ErrNotFound)
}
