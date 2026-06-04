package domain

// CanConfirm reports whether a sub-order in the given status may be confirmed.
func CanConfirm(s SubOrderStatus) bool {
	return s == SubOrderStatusPending
}

// CanShip reports whether a confirmed sub-order may be shipped. The parent order
// must be in 'processing' (PayOS paid, or COD which is processing from placement).
func CanShip(s SubOrderStatus, orderStatus OrderStatus) bool {
	return s == SubOrderStatusConfirmed && orderStatus == OrderStatusProcessing
}
