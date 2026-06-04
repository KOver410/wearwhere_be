package goship

import "strings"

// DeliveryCategory is the coarse outcome a webhook maps to.
type DeliveryCategory int

const (
	CategoryShipped   DeliveryCategory = iota // in-transit / picked up / waiting pickup
	CategoryDelivered                         // successfully delivered
	CategoryOther                             // return / lost / unknown-terminal — record text only
)

// deliveredHints are matched case-insensitively against status_text as a fallback
// while numeric codes aren't fully catalogued.
var deliveredHints = []string{"đã giao", "giao thành công", "delivered", "thành công"}

// MapStatus maps a Goship webhook status to a coarse category.
// Returned/lost shipments are CategoryOther (recorded, not auto-restocked — out of Spec B scope).
// Confirmed code "901" = Chờ lấy hàng (waiting pickup) -> Shipped. Extend codes here as observed.
func MapStatus(status, statusText string, isReturn, isLost int) DeliveryCategory {
	if isReturn == 1 || isLost == 1 {
		return CategoryOther
	}
	t := strings.ToLower(statusText)
	for _, h := range deliveredHints {
		if strings.Contains(t, h) {
			return CategoryDelivered
		}
	}
	return CategoryShipped
}
