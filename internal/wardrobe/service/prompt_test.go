package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/wardrobe/service"
)

func TestParseOutfits_PlainAndFenced(t *testing.T) {
	plain := `{"outfits":[{"title":"A","note":"n","item_ids":["1","2"]}]}`
	got, err := service.ParseOutfits(plain)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, []string{"1", "2"}, got[0].ItemIDs)

	fenced := "```json\n" + plain + "\n```"
	got, err = service.ParseOutfits(fenced)
	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestParseOutfits_GarbageErrors(t *testing.T) {
	_, err := service.ParseOutfits("not json at all")
	require.Error(t, err)
}
