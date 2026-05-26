// internal/order/domain/order_no.go
package domain

import (
	"crypto/rand"
	"fmt"
	"time"
)

// nanoidAlphabet excludes I, O, 0, 1 to avoid visual confusion.
const nanoidAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// GenerateOrderNo returns a string like "WW-20260524-AB12CD".
// Uniqueness is enforced by DB unique constraint; caller retries on conflict.
func GenerateOrderNo(now time.Time) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand failure is fatal; we'd rather crash than return predictable IDs
		panic(fmt.Sprintf("order_no: crypto/rand failed: %v", err))
	}
	suffix := make([]byte, 6)
	for i, b := range buf {
		suffix[i] = nanoidAlphabet[int(b)%len(nanoidAlphabet)]
	}
	return fmt.Sprintf("WW-%s-%s", now.Format("20060102"), string(suffix))
}
