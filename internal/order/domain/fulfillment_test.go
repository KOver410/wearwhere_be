package domain

import "testing"

func TestCanConfirm(t *testing.T) {
	if !CanConfirm(SubOrderStatusPending) {
		t.Error("pending should be confirmable")
	}
	if CanConfirm(SubOrderStatusConfirmed) {
		t.Error("confirmed should NOT be re-confirmable")
	}
}

func TestCanShip(t *testing.T) {
	if !CanShip(SubOrderStatusConfirmed, OrderStatusProcessing) {
		t.Error("confirmed + processing should be shippable")
	}
	if CanShip(SubOrderStatusPending, OrderStatusProcessing) {
		t.Error("pending should not be shippable")
	}
	if CanShip(SubOrderStatusConfirmed, OrderStatusPendingPayment) {
		t.Error("unpaid order should not be shippable")
	}
}
