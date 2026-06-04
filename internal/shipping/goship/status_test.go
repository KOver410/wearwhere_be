package goship

import "testing"

func TestMapStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		text     string
		isReturn int
		isLost   int
		want     DeliveryCategory
	}{
		{"waiting pickup 901", "901", "Chờ lấy hàng", 0, 0, CategoryShipped},
		{"delivered text", "", "Đã giao hàng", 0, 0, CategoryDelivered},
		{"returned flag", "", "Hoàn hàng", 1, 0, CategoryOther},
		{"lost flag", "", "Thất lạc", 0, 1, CategoryOther},
		{"unknown -> shipped", "555", "Đang vận chuyển", 0, 0, CategoryShipped},
		{"pickup success not delivered", "", "Lấy hàng thành công", 0, 0, CategoryShipped},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MapStatus(tc.status, tc.text, tc.isReturn, tc.isLost); got != tc.want {
				t.Errorf("MapStatus(%q,%q,%d,%d) = %v, want %v", tc.status, tc.text, tc.isReturn, tc.isLost, got, tc.want)
			}
		})
	}
}
