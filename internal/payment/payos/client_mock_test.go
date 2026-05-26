// internal/payment/payos/client_mock_test.go
package payos_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
)

func TestMockClient_CreateLink_ReturnsLocalURL(t *testing.T) {
	m := payos.NewMockClient("http://localhost:8080")
	r, err := m.CreateLink(context.Background(), payos.CreateLinkReq{
		OrderCode: 42, AmountVND: 130000, Description: "test",
	})
	require.NoError(t, err)
	require.Equal(t, "http://localhost:8080/dev/payos/mock-checkout?orderCode=42", r.CheckoutURL)
	require.Equal(t, int64(42), r.OrderCode)
}

func TestMockClient_VerifyWebhook_AlwaysOK(t *testing.T) {
	m := payos.NewMockClient("")
	require.NoError(t, m.VerifyWebhookSignature(payos.WebhookPayload{Signature: "anything"}))
}

func TestFactory_DefaultsMock(t *testing.T) {
	c, err := payos.NewFromConfig(payos.Config{Mode: ""})
	require.NoError(t, err)
	require.NotNil(t, c)
}

func TestFactory_ProductionRequiresCreds(t *testing.T) {
	_, err := payos.NewFromConfig(payos.Config{Mode: "production"})
	require.Error(t, err)
}

func TestFactory_UnknownMode(t *testing.T) {
	_, err := payos.NewFromConfig(payos.Config{Mode: "stripe"})
	require.Error(t, err)
}
