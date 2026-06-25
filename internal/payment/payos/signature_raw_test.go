package payos

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// payosSign replicates how PayOS builds the signature base string: ALL data
// fields, sorted by key, joined key=value&..., then HMAC-SHA256.
func payosSign(checksum string, sortedPairs string) string {
	mac := hmac.New(sha256.New, []byte(checksum))
	mac.Write([]byte(sortedPairs))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyWebhookRaw_AcceptsFullPayosFieldSet(t *testing.T) {
	const checksum = "test-checksum-key"

	// A realistic PayOS webhook `data` object — note the 6 counterAccount*/
	// virtualAccount* fields the typed struct does NOT model. PayOS signs them.
	rawData := []byte(`{` +
		`"orderCode":123,"amount":3000,"description":"VQRIO123",` +
		`"accountNumber":"12345678","reference":"TF230204212323",` +
		`"transactionDateTime":"2023-02-04 18:25:00","currency":"VND",` +
		`"paymentLinkId":"124c33293c43417ab7879e14c8d9eb18","code":"00","desc":"success",` +
		`"counterAccountBankId":"","counterAccountBankName":"","counterAccountName":"",` +
		`"counterAccountNumber":"","virtualAccountName":"","virtualAccountNumber":""}`)

	// Signature base string: keys sorted alphabetically, all 16 fields included.
	base := "accountNumber=12345678&amount=3000&code=00" +
		"&counterAccountBankId=&counterAccountBankName=&counterAccountName=&counterAccountNumber=" +
		"&currency=VND&desc=success&description=VQRIO123&orderCode=123" +
		"&paymentLinkId=124c33293c43417ab7879e14c8d9eb18&reference=TF230204212323" +
		"&transactionDateTime=2023-02-04 18:25:00&virtualAccountName=&virtualAccountNumber="
	sig := payosSign(checksum, base)

	if err := VerifyWebhookRaw(checksum, rawData, sig); err != nil {
		t.Fatalf("expected valid signature to pass, got %v", err)
	}

	// Tampered signature must be rejected.
	if err := VerifyWebhookRaw(checksum, rawData, sig[:len(sig)-1]+"0"); err == nil {
		t.Fatal("expected tampered signature to be rejected")
	}

	// Regression guard: the old typed-subset verifier (10 fields) cannot validate
	// a signature computed over the full 16-field set — proving why we sign raw.
	var p WebhookPayload
	p.Signature = sig
	p.Data = WebhookData{OrderCode: 123, Amount: 3000, Description: "VQRIO123",
		AccountNumber: "12345678", Reference: "TF230204212323",
		TransactionDateTime: "2023-02-04 18:25:00", Currency: "VND",
		PaymentLinkID: "124c33293c43417ab7879e14c8d9eb18", Code: "00", Desc: "success"}
	if err := VerifyWebhook(checksum, p); err == nil {
		t.Fatal("typed-subset verifier should fail on full-field PayOS signature")
	}
}
