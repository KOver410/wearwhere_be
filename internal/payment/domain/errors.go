// internal/payment/domain/errors.go
package domain

import "errors"

var (
	ErrPaymentNotFound = errors.New("payment: not found")
	ErrIdempotent      = errors.New("payment: already processed (idempotent)")
)
