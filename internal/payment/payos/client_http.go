// internal/payment/payos/client_http.go
package payos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const payosBaseURL = "https://api-merchant.payos.vn"

type HTTPClient struct {
	clientID    string
	apiKey      string
	checksumKey string
	httpClient  *http.Client
	baseURL     string
}

func NewHTTPClient(clientID, apiKey, checksumKey string) *HTTPClient {
	return &HTTPClient{
		clientID:    clientID,
		apiKey:      apiKey,
		checksumKey: checksumKey,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		baseURL:     payosBaseURL,
	}
}

type payosEnvelope struct {
	Code string          `json:"code"`
	Desc string          `json:"desc"`
	Data json.RawMessage `json:"data"`
}

func (c *HTTPClient) CreateLink(ctx context.Context, r CreateLinkReq) (*CreateLinkResp, error) {
	// Build request body and signature.
	body := map[string]any{
		"orderCode":   r.OrderCode,
		"amount":      r.AmountVND,
		"description": r.Description,
		"items":       r.Items,
		"returnUrl":   r.ReturnURL,
		"cancelUrl":   r.CancelURL,
		"buyerName":   r.Buyer.Name,
		"buyerPhone":  r.Buyer.Phone,
		"buyerEmail":  r.Buyer.Email,
		"expiredAt":   r.ExpiredAt,
	}
	// Signature for create-link: amount, cancelUrl, description, orderCode, returnUrl
	body["signature"] = Sign(c.checksumKey, map[string]any{
		"amount":      r.AmountVND,
		"cancelUrl":   r.CancelURL,
		"description": r.Description,
		"orderCode":   r.OrderCode,
		"returnUrl":   r.ReturnURL,
	})

	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/v2/payment-requests", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-client-id", c.clientID)
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCreateLink, err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: status=%d body=%s", ErrCreateLink, resp.StatusCode, string(bodyBytes))
	}

	var env payosEnvelope
	if err := json.Unmarshal(bodyBytes, &env); err != nil {
		return nil, err
	}
	if env.Code != "00" {
		return nil, fmt.Errorf("%w: code=%s desc=%s", ErrCreateLink, env.Code, env.Desc)
	}

	var data struct {
		PaymentLinkID string `json:"paymentLinkId"`
		CheckoutURL   string `json:"checkoutUrl"`
		QRCode        string `json:"qrCode"`
		OrderCode     int64  `json:"orderCode"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return nil, err
	}

	return &CreateLinkResp{
		PaymentLinkID: data.PaymentLinkID,
		CheckoutURL:   data.CheckoutURL,
		QRCode:        data.QRCode,
		OrderCode:     data.OrderCode,
		ExpiredAt:     time.Unix(r.ExpiredAt, 0),
	}, nil
}

func (c *HTTPClient) VerifyWebhookSignature(p WebhookPayload) error {
	return VerifyWebhook(c.checksumKey, p)
}

func (c *HTTPClient) VerifyWebhookSignatureRaw(rawData []byte, signature string) error {
	return VerifyWebhookRaw(c.checksumKey, rawData, signature)
}

func (c *HTTPClient) GetPayment(ctx context.Context, paymentLinkID string) (*PaymentInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		c.baseURL+"/v2/payment-requests/"+paymentLinkID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-client-id", c.clientID)
	req.Header.Set("x-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("payos GetPayment: status=%d body=%s", resp.StatusCode, string(bodyBytes))
	}

	var env payosEnvelope
	if err := json.Unmarshal(bodyBytes, &env); err != nil {
		return nil, err
	}
	var data struct {
		OrderCode int64  `json:"orderCode"`
		Status    string `json:"status"`
		Amount    int64  `json:"amount"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return nil, err
	}

	return &PaymentInfo{OrderCode: data.OrderCode, Status: data.Status, Amount: data.Amount}, nil
}

func (c *HTTPClient) CancelLink(ctx context.Context, paymentLinkID, reason string) error {
	body := map[string]any{"cancellationReason": reason}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/v2/payment-requests/"+paymentLinkID+"/cancel", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("x-client-id", c.clientID)
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("payos CancelLink: status=%d body=%s", resp.StatusCode, string(b))
	}
	return nil
}
