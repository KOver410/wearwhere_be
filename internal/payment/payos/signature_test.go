// internal/payment/payos/signature_test.go
package payos_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
)

func TestSign_DeterministicAcrossInputOrder(t *testing.T) {
	a := payos.Sign("key", map[string]any{"b": 2, "a": 1, "c": 3})
	b := payos.Sign("key", map[string]any{"c": 3, "a": 1, "b": 2})
	require.Equal(t, a, b)
}

func TestSign_KnownValue(t *testing.T) {
	// Hand-computed: HMAC-SHA256("secret", "a=1&b=2") -> compare via stdlib
	expected := func() string {
		h := hmac.New(sha256.New, []byte("secret"))
		h.Write([]byte("a=1&b=2"))
		return hex.EncodeToString(h.Sum(nil))
	}()
	got := payos.Sign("secret", map[string]any{"a": 1, "b": 2})
	require.Equal(t, expected, got)
}

func TestVerifyWebhook_ValidSignature(t *testing.T) {
	data := payos.WebhookData{
		OrderCode: 12345, Amount: 100000, Description: "test",
		AccountNumber: "vcb1", Reference: "ref1", TransactionDateTime: "2026-05-24",
		Currency: "VND", PaymentLinkID: "pl1", Code: "00", Desc: "Success",
	}
	fields := map[string]any{
		"orderCode": data.OrderCode, "amount": data.Amount, "description": data.Description,
		"accountNumber": data.AccountNumber, "reference": data.Reference,
		"transactionDateTime": data.TransactionDateTime, "currency": data.Currency,
		"paymentLinkId": data.PaymentLinkID, "code": data.Code, "desc": data.Desc,
	}
	sig := payos.Sign("secret-key", fields)

	err := payos.VerifyWebhook("secret-key", payos.WebhookPayload{
		Code: "00", Success: true, Data: data, Signature: sig,
	})
	require.NoError(t, err)
}

func TestVerifyWebhook_InvalidSignature(t *testing.T) {
	err := payos.VerifyWebhook("secret-key", payos.WebhookPayload{
		Data:      payos.WebhookData{OrderCode: 1, Amount: 100},
		Signature: "deadbeef",
	})
	require.ErrorIs(t, err, payos.ErrSignatureInvalid)
}
