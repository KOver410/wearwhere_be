//go:build payos_real

// Real PayOS integration tests. These hit the LIVE PayOS API and require
// valid credentials in env:
//
//	PAYOS_CLIENT_ID, PAYOS_API_KEY, PAYOS_CHECKSUM_KEY
//
// Tests are gated by the `payos_real` build tag so `go test ./...` does NOT
// trigger them by default. Run explicitly:
//
//	go test -tags payos_real ./internal/payment/payos/ -v -count=1
//
// Each test cleans up after itself (cancels the payment link it created).
// Tests skip if credentials are not set so the file is safe to keep in the repo.
package payos_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
)

// loadPayosEnvOrSkip reads credentials from .env / process env. Skips the test
// if any of the three required vars is empty.
func loadPayosEnvOrSkip(t *testing.T) (clientID, apiKey, checksumKey string) {
	t.Helper()
	_ = godotenv.Load("../../../.env") // best-effort; CI may inject via env instead

	clientID = os.Getenv("PAYOS_CLIENT_ID")
	apiKey = os.Getenv("PAYOS_API_KEY")
	checksumKey = os.Getenv("PAYOS_CHECKSUM_KEY")
	if clientID == "" || apiKey == "" || checksumKey == "" {
		t.Skip("PAYOS_CLIENT_ID / PAYOS_API_KEY / PAYOS_CHECKSUM_KEY not set — skipping real PayOS tests")
	}
	return
}

// uniqueOrderCode returns a millisecond-resolution positive int64 PayOS will
// accept. Each test uses a fresh one so re-runs do not conflict server-side.
func uniqueOrderCode() int64 {
	return time.Now().UnixNano() / int64(time.Microsecond) // ~16-digit positive int64
}

// TestRealPayos_CreateLink_ThenGet_ThenCancel exercises the full HTTP roundtrip
// against the live PayOS API. It creates a real payment link, queries it back,
// then cancels it for cleanup.
func TestRealPayos_CreateLink_ThenGet_ThenCancel(t *testing.T) {
	clientID, apiKey, checksumKey := loadPayosEnvOrSkip(t)
	client := payos.NewHTTPClient(clientID, apiKey, checksumKey)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	code := uniqueOrderCode()
	req := payos.CreateLinkReq{
		OrderCode:   code,
		AmountVND:   10000, // smallest tier acceptable for PayOS test orders
		Description: "WW Test Order",
		Items: []payos.LineItem{
			{Name: "Test Tee", Quantity: 1, Price: 10000},
		},
		ReturnURL: "http://localhost:3000/checkout/success",
		CancelURL: "http://localhost:3000/checkout/cancel",
		Buyer: payos.Buyer{
			Name:  "WearWhere QA",
			Phone: "0900000000",
			Email: "qa@wearwhere.local",
		},
		ExpiredAt: time.Now().Add(15 * time.Minute).Unix(),
	}

	resp, err := client.CreateLink(ctx, req)
	require.NoError(t, err, "CreateLink against real PayOS must succeed")
	require.NotEmpty(t, resp.PaymentLinkID, "PaymentLinkID must come back from PayOS")
	require.NotEmpty(t, resp.CheckoutURL, "CheckoutURL must come back from PayOS")
	require.Equal(t, code, resp.OrderCode, "orderCode must roundtrip")

	t.Logf("PayOS created link: id=%s  url=%s  expiredAt=%s",
		resp.PaymentLinkID, resp.CheckoutURL, resp.ExpiredAt.Format(time.RFC3339))

	// Always attempt cleanup, even if the next assertion fails.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if cancelErr := client.CancelLink(cleanupCtx, resp.PaymentLinkID, "test cleanup"); cancelErr != nil {
			t.Logf("CancelLink cleanup failed (non-fatal): %v", cancelErr)
		}
	})

	// GetPayment should return the link we just created, status PENDING.
	info, err := client.GetPayment(ctx, resp.PaymentLinkID)
	require.NoError(t, err, "GetPayment must succeed for a just-created link")
	require.Equal(t, code, info.OrderCode, "GetPayment orderCode must match")
	require.NotEmpty(t, info.Status, "GetPayment must surface a status")
	t.Logf("PayOS GetPayment: orderCode=%d status=%s amount=%d",
		info.OrderCode, info.Status, info.Amount)
}

// TestRealPayos_CancelLink directly verifies cancellation on a freshly created link.
func TestRealPayos_CancelLink(t *testing.T) {
	clientID, apiKey, checksumKey := loadPayosEnvOrSkip(t)
	client := payos.NewHTTPClient(clientID, apiKey, checksumKey)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	code := uniqueOrderCode()
	resp, err := client.CreateLink(ctx, payos.CreateLinkReq{
		OrderCode:   code,
		AmountVND:   10000,
		Description: "WW Cancel Test",
		Items:       []payos.LineItem{{Name: "Cancel Tee", Quantity: 1, Price: 10000}},
		ReturnURL:   "http://localhost:3000/checkout/success",
		CancelURL:   "http://localhost:3000/checkout/cancel",
		Buyer:       payos.Buyer{Name: "QA", Phone: "0900000000", Email: "qa@wearwhere.local"},
		ExpiredAt:   time.Now().Add(15 * time.Minute).Unix(),
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.PaymentLinkID)

	require.NoError(t, client.CancelLink(ctx, resp.PaymentLinkID, "explicit cancel test"))

	// After cancellation PayOS typically returns status=CANCELLED.
	info, err := client.GetPayment(ctx, resp.PaymentLinkID)
	require.NoError(t, err)
	t.Logf("PayOS post-cancel status: %s", info.Status)
}

// TestRealPayos_SignatureMatchesReferenceHMAC proves our Sign() implementation
// produces the exact byte-for-byte HMAC PayOS would compute, using the LIVE
// checksum key from env. This is the strongest local verification short of
// having PayOS POST a real webhook to us.
func TestRealPayos_SignatureMatchesReferenceHMAC(t *testing.T) {
	_, _, checksumKey := loadPayosEnvOrSkip(t)

	// Use the same field set PayOS includes in webhook `data`.
	data := payos.WebhookData{
		OrderCode:           1234567890,
		Amount:              50000,
		Description:         "DH WW-20260526-AB12CD",
		AccountNumber:       "12345678",
		Reference:           "ref-abc",
		TransactionDateTime: "2026-05-26 10:00:00",
		Currency:            "VND",
		PaymentLinkID:       "abc-link-id",
		Code:                "00",
		Desc:                "Success",
	}

	fields := map[string]any{
		"orderCode":           data.OrderCode,
		"amount":              data.Amount,
		"description":         data.Description,
		"accountNumber":       data.AccountNumber,
		"reference":           data.Reference,
		"transactionDateTime": data.TransactionDateTime,
		"currency":            data.Currency,
		"paymentLinkId":       data.PaymentLinkID,
		"code":                data.Code,
		"desc":                data.Desc,
	}

	ourSig := payos.Sign(checksumKey, fields)

	// Independent HMAC computation using stdlib only — must match.
	// Expected concat order (alphabetical):
	//   accountNumber, amount, code, currency, desc, description, orderCode,
	//   paymentLinkId, reference, transactionDateTime
	expectedConcat := "accountNumber=" + data.AccountNumber +
		"&amount=50000" +
		"&code=" + data.Code +
		"&currency=" + data.Currency +
		"&desc=" + data.Desc +
		"&description=" + data.Description +
		"&orderCode=1234567890" +
		"&paymentLinkId=" + data.PaymentLinkID +
		"&reference=" + data.Reference +
		"&transactionDateTime=" + data.TransactionDateTime
	h := hmac.New(sha256.New, []byte(checksumKey))
	h.Write([]byte(expectedConcat))
	expectedSig := hex.EncodeToString(h.Sum(nil))

	require.Equal(t, expectedSig, ourSig,
		"Sign() must produce the same HMAC PayOS would compute over alphabetically-sorted fields")

	// And VerifyWebhook must accept our own signature against the same data.
	require.NoError(t, payos.VerifyWebhook(checksumKey, payos.WebhookPayload{
		Code:      "00",
		Success:   true,
		Data:      data,
		Signature: ourSig,
	}))
}

// TestRealPayos_VerifyWebhook_TamperedSignature ensures we reject any payload
// whose signature was forged with the wrong key — protects against attackers
// who guess the orderCode without knowing the checksum.
func TestRealPayos_VerifyWebhook_TamperedSignature(t *testing.T) {
	_, _, realChecksum := loadPayosEnvOrSkip(t)

	data := payos.WebhookData{
		OrderCode: 99999999,
		Amount:    50000,
		Code:      "00",
		Currency:  "VND",
	}
	forged := payos.Sign("not-the-real-key", map[string]any{
		"orderCode": data.OrderCode,
		"amount":    data.Amount,
		"code":      data.Code,
		"currency":  data.Currency,
	})
	err := payos.VerifyWebhook(realChecksum, payos.WebhookPayload{
		Data:      data,
		Signature: forged,
	})
	require.ErrorIs(t, err, payos.ErrSignatureInvalid)
}
