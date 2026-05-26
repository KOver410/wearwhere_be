// internal/order/domain/order_no_test.go
package domain_test

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
)

func TestGenerateOrderNo_Format(t *testing.T) {
	now := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	no := domain.GenerateOrderNo(now)
	re := regexp.MustCompile(`^WW-20260524-[A-Z2-9]{6}$`)
	require.True(t, re.MatchString(no), "got %q", no)
}

func TestGenerateOrderNo_ExcludesAmbiguousChars(t *testing.T) {
	now := time.Now()
	// Statistical: generate 1000, none should contain I, O, 0, 1.
	for i := 0; i < 1000; i++ {
		no := domain.GenerateOrderNo(now)
		for _, ch := range no[12:] { // skip "WW-YYYYMMDD-"
			require.NotEqual(t, 'I', ch, "found I in %q", no)
			require.NotEqual(t, 'O', ch, "found O in %q", no)
			require.NotEqual(t, '0', ch, "found 0 in %q", no)
			require.NotEqual(t, '1', ch, "found 1 in %q", no)
		}
	}
}

func TestGenerateOrderNo_UniqueAcrossManyCalls(t *testing.T) {
	now := time.Now()
	seen := make(map[string]struct{})
	for i := 0; i < 10000; i++ {
		no := domain.GenerateOrderNo(now)
		_, dup := seen[no]
		require.False(t, dup, "duplicate %q at iter %d", no, i)
		seen[no] = struct{}{}
	}
}
