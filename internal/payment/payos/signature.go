// internal/payment/payos/signature.go
package payos

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Sign computes HMAC-SHA256 over sorted key=value pairs joined by '&'.
// Per PayOS spec: keys sorted alphabetically; values stringified (numbers without quotes).
func Sign(checksumKey string, fields map[string]any) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('&')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(fmt.Sprintf("%v", fields[k]))
	}
	mac := hmac.New(sha256.New, []byte(checksumKey))
	mac.Write([]byte(sb.String()))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyWebhook checks a webhook payload's signature against checksumKey.
// Returns ErrSignatureInvalid on mismatch.
func VerifyWebhook(checksumKey string, p WebhookPayload) error {
	fields := webhookDataToMap(p.Data)
	expected := Sign(checksumKey, fields)
	if !hmac.Equal([]byte(expected), []byte(p.Signature)) {
		return ErrSignatureInvalid
	}
	return nil
}

func webhookDataToMap(d WebhookData) map[string]any {
	return map[string]any{
		"orderCode":           d.OrderCode,
		"amount":              d.Amount,
		"description":         d.Description,
		"accountNumber":       d.AccountNumber,
		"reference":           d.Reference,
		"transactionDateTime": d.TransactionDateTime,
		"currency":            d.Currency,
		"paymentLinkId":       d.PaymentLinkID,
		"code":                d.Code,
		"desc":                d.Desc,
	}
}
