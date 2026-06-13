package domain

import (
	"testing"
	"time"
)

func TestHaversineKm_KnownDistance(t *testing.T) {
	d := HaversineKm(10.7720, 106.6980, 10.7951, 106.7218)
	if d < 3 || d > 5 {
		t.Errorf("HaversineKm = %.2f, want roughly 3-5km", d)
	}
}

func TestComputeOpenStatus_OpenInsideWindow(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, loc) // Monday 10:00, weekday=1
	hours := []StoreHours{{Weekday: 1, OpenTime: "09:00", CloseTime: "21:00"}}
	st := ComputeOpenStatus(hours, now)
	if st == nil || !st.Open {
		t.Fatalf("expected open, got %+v", st)
	}
}

func TestComputeOpenStatus_ClosedOutsideWindow(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	now := time.Date(2026, 6, 15, 22, 0, 0, 0, loc) // Monday 22:00
	hours := []StoreHours{{Weekday: 1, OpenTime: "09:00", CloseTime: "21:00"}}
	st := ComputeOpenStatus(hours, now)
	if st == nil || st.Open {
		t.Fatalf("expected closed, got %+v", st)
	}
}

func TestComputeOpenStatus_NoHoursReturnsNil(t *testing.T) {
	if ComputeOpenStatus(nil, time.Now()) != nil {
		t.Error("expected nil open status when no hours configured")
	}
}
