//go:build integration

package repo

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/store/domain"
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

// seedStore inserts a public, geocoded brand_address (a "store") and returns its id.
func seedStore(t *testing.T, db testfixtures.DBTX, brandID uuid.UUID, lat, lng float64, cityCode string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := db.Exec(context.Background(),
		`INSERT INTO brand_addresses
		   (id, brand_id, label, address_line, ward, district, city,
		    city_code, country, latitude, longitude, is_primary, is_public)
		 VALUES ($1,$2,'Cửa hàng','1 Test St','Phường X','Quận Y','TP HCM',
		         $3,'VN',$4,$5,FALSE,TRUE)`,
		id, brandID, cityCode, lat, lng)
	require.NoError(t, err)
	return id
}

func TestStorePG_Nearby_OrdersByDistance(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	ctx := context.Background()
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)

	const oLat, oLng = 10.7769, 106.7009
	near := seedStore(t, tx, sb.ID, 10.7800, 106.7000, "79") // ~0.5km
	far := seedStore(t, tx, sb.ID, 10.8200, 106.7400, "79")  // ~6km

	r := NewStorePG(tx)
	got, err := r.Nearby(ctx, oLat, oLng, 10, 25)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, near, got[0].AddressID, "nearest first")
	require.Equal(t, far, got[1].AddressID, "farthest last")

	// Sanity: returned distances (haversine in Go) are ascending.
	d0 := domain.HaversineKm(oLat, oLng, got[0].Latitude, got[0].Longitude)
	d1 := domain.HaversineKm(oLat, oLng, got[1].Latitude, got[1].Longitude)
	require.LessOrEqual(t, d0, d1)
}

func TestStorePG_Nearby_ExcludesOutsideRadius(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	ctx := context.Background()
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	seedStore(t, tx, sb.ID, 10.7800, 106.7000, "79") // ~0.5km in
	seedStore(t, tx, sb.ID, 21.0285, 105.8542, "01") // Hanoi, far out

	r := NewStorePG(tx)
	got, err := r.Nearby(ctx, 10.7769, 106.7009, 5, 25)
	require.NoError(t, err)
	require.Len(t, got, 1, "only the in-radius HCMC store")
}

func TestStorePG_SearchByArea_ByCityCode(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	ctx := context.Background()
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	want := seedStore(t, tx, sb.ID, 10.78, 106.70, "79")
	seedStore(t, tx, sb.ID, 21.02, 105.85, "01") // different city

	r := NewStorePG(tx)
	got, err := r.SearchByArea(ctx, AreaFilter{CityCode: "79", Limit: 50})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, want, got[0].AddressID)
}

func TestStorePG_Detail_WithHours(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	ctx := context.Background()
	sb := testfixtures.SeedBrand(t, tx, uuid.Nil)
	id := seedStore(t, tx, sb.ID, 10.78, 106.70, "79")
	_, err := tx.Exec(ctx,
		`INSERT INTO store_hours (brand_address_id, weekday, open_time, close_time)
		 VALUES ($1, 1, TIME '09:00', TIME '21:00')`, id)
	require.NoError(t, err)

	r := NewStorePG(tx)
	got, err := r.Detail(ctx, id)
	require.NoError(t, err)
	require.Equal(t, sb.Name, got.BrandName)
	require.Len(t, got.Hours, 1)
	require.Equal(t, "09:00", got.Hours[0].OpenTime)
	require.Equal(t, "21:00", got.Hours[0].CloseTime)
}

func TestStorePG_Detail_NotFound(t *testing.T) {
	tx := testfixtures.BeginTx(t, testPool)
	r := NewStorePG(tx)
	_, err := r.Detail(context.Background(), uuid.New())
	require.ErrorIs(t, err, ErrNotFound)
}
