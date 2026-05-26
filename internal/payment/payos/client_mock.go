// internal/payment/payos/client_mock.go
package payos

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

type MockClient struct {
	seq     atomic.Int64
	baseURL string // for constructing mock checkout URL, e.g. http://localhost:8080
}

func NewMockClient(baseURL string) *MockClient {
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	return &MockClient{baseURL: baseURL}
}

func (m *MockClient) CreateLink(_ context.Context, r CreateLinkReq) (*CreateLinkResp, error) {
	id := fmt.Sprintf("mock-pl-%d", m.seq.Add(1))
	return &CreateLinkResp{
		PaymentLinkID: id,
		CheckoutURL:   fmt.Sprintf("%s/dev/payos/mock-checkout?orderCode=%d", m.baseURL, r.OrderCode),
		QRCode:        "data:image/png;base64,mock-qr",
		OrderCode:     r.OrderCode,
		ExpiredAt:     time.Unix(r.ExpiredAt, 0),
	}, nil
}

// Mock accepts any signature — testing convenience only.
func (m *MockClient) VerifyWebhookSignature(_ WebhookPayload) error { return nil }

func (m *MockClient) GetPayment(_ context.Context, paymentLinkID string) (*PaymentInfo, error) {
	return &PaymentInfo{Status: "PENDING"}, nil
}

func (m *MockClient) CancelLink(_ context.Context, _, _ string) error { return nil }
