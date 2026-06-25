// internal/payment/payos/signature.go
package payos

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// VerifyWebhookRaw verifies a PayOS webhook signature against the `data` object
// exactly as received on the wire. PayOS signs EVERY field present in `data`
// (sorted by key, joined as key=value&...), including ones our typed struct does
// not model (counterAccount*, virtualAccount*). Reconstructing the signature from
// a fixed struct subset therefore fails verification — so we sign the received
// fields verbatim, mirroring PayOS's own SDKs.
func VerifyWebhookRaw(checksumKey string, rawData []byte, signature string) error {
	dec := json.NewDecoder(bytes.NewReader(rawData))
	dec.UseNumber() // keep large ints (orderCode) exact; never float64
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return ErrSignatureInvalid
	}

	keys := make([]string, 0, len(m))
	for k := range m {
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
		sb.WriteString(payosFieldValue(m[k]))
	}
	mac := hmac.New(sha256.New, []byte(checksumKey))
	mac.Write([]byte(sb.String()))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return ErrSignatureInvalid
	}
	return nil
}

// payosFieldValue stringifies a JSON value the way PayOS does when building the
// signature base string: null/"null"/"undefined" → empty; numbers verbatim;
// bools as true/false; nested values JSON-encoded.
func payosFieldValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		if x == "null" || x == "undefined" {
			return ""
		}
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case json.Number:
		return x.String()
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}
