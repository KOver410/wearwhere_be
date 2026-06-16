package service_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	spdomain "github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
	"github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
	"github.com/wearwhere/wearwhere_be/internal/wardrobe/service"
)

func TestComputeSignature_StableAndSensitive(t *testing.T) {
	c1 := []domain.ClosetItem{{ProductID: uuid.New()}}
	prof := &spdomain.StyleProfileView{StyleTags: []spdomain.StyleTagRef{{ID: uuid.New().String()}}}

	s1 := service.ComputeSignature(c1, prof, "20260616")
	s2 := service.ComputeSignature(c1, prof, "20260616")
	require.Equal(t, s1, s2, "same inputs → same signature")

	// Different closet → different signature.
	c2 := []domain.ClosetItem{{ProductID: uuid.New()}}
	require.NotEqual(t, s1, service.ComputeSignature(c2, prof, "20260616"))

	// Different profile → different signature.
	prof2 := &spdomain.StyleProfileView{StyleTags: []spdomain.StyleTagRef{{ID: uuid.New().String()}}}
	require.NotEqual(t, s1, service.ComputeSignature(c1, prof2, "20260616"))
}

func TestComputeSignature_EmptyClosetVariesByDay(t *testing.T) {
	s1 := service.ComputeSignature(nil, nil, "20260616")
	s2 := service.ComputeSignature(nil, nil, "20260617")
	require.NotEqual(t, s1, s2, "empty closet refreshes daily")
}
