// internal/payment/payos/client.go
package payos

import (
	"context"
	"errors"
	"time"
)

var (
	ErrSignatureInvalid = errors.New("payos: invalid webhook signature")
	ErrCreateLink       = errors.New("payos: failed to create payment link")
)

type LineItem struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
	Price    int64  `json:"price"`
}

type Buyer struct {
	Name  string `json:"buyerName"`
	Phone string `json:"buyerPhone"`
	Email string `json:"buyerEmail"`
}

type CreateLinkReq struct {
	OrderCode   int64      `json:"orderCode"`
	AmountVND   int64      `json:"amount"`
	Description string     `json:"description"`
	Items       []LineItem `json:"items"`
	ReturnURL   string     `json:"returnUrl"`
	CancelURL   string     `json:"cancelUrl"`
	Buyer       Buyer      `json:",inline"`
	ExpiredAt   int64      `json:"expiredAt"`
}

type CreateLinkResp struct {
	PaymentLinkID string    `json:"paymentLinkId"`
	CheckoutURL   string    `json:"checkoutUrl"`
	QRCode        string    `json:"qrCode"`
	OrderCode     int64     `json:"orderCode"`
	ExpiredAt     time.Time `json:"-"`
}

type WebhookData struct {
	OrderCode           int64  `json:"orderCode"`
	Amount              int64  `json:"amount"`
	Description         string `json:"description"`
	AccountNumber       string `json:"accountNumber"`
	Reference           string `json:"reference"`
	TransactionDateTime string `json:"transactionDateTime"`
	Currency            string `json:"currency"`
	PaymentLinkID       string `json:"paymentLinkId"`
	Code                string `json:"code"`
	Desc                string `json:"desc"`
}

type WebhookPayload struct {
	Code      string      `json:"code"`
	Desc      string      `json:"desc"`
	Success   bool        `json:"success"`
	Data      WebhookData `json:"data"`
	Signature string      `json:"signature"`
}

type PaymentInfo struct {
	OrderCode int64
	Status    string
	Amount    int64
}

type Client interface {
	CreateLink(ctx context.Context, r CreateLinkReq) (*CreateLinkResp, error)
	VerifyWebhookSignature(p WebhookPayload) error
	GetPayment(ctx context.Context, paymentLinkID string) (*PaymentInfo, error)
	CancelLink(ctx context.Context, paymentLinkID, reason string) error
}
