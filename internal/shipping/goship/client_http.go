package goship

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type HTTPClient struct {
	token        string
	clientSecret string
	baseURL      string
	httpClient   *http.Client
}

func NewHTTPClient(token, clientSecret, baseURL string) *HTTPClient {
	return &HTTPClient{
		token:        token,
		clientSecret: clientSecret,
		baseURL:      baseURL,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *HTTPClient) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("goship %s %s: status=%d body=%s", method, path, resp.StatusCode, string(b))
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

// locationEnvelope matches Goship's { "data": [ { "id": "100000", "name": "..." } ] }.
// id may arrive as a JSON string or number depending on the endpoint, so decode
// it as json.Number and stringify.
type locationEnvelope struct {
	Data []struct {
		ID   json.Number `json:"id"`
		Name string      `json:"name"`
	} `json:"data"`
}

func (c *HTTPClient) locations(ctx context.Context, path string) ([]Location, error) {
	var env locationEnvelope
	if err := c.do(ctx, http.MethodGet, path, nil, &env); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLocation, err)
	}
	out := make([]Location, 0, len(env.Data))
	for _, d := range env.Data {
		out = append(out, Location{Code: d.ID.String(), Name: d.Name})
	}
	return out, nil
}

func (c *HTTPClient) Cities(ctx context.Context) ([]Location, error) {
	return c.locations(ctx, "/cities")
}

func (c *HTTPClient) Districts(ctx context.Context, cityCode string) ([]Location, error) {
	return c.locations(ctx, "/cities/"+cityCode+"/districts")
}

func (c *HTTPClient) Wards(ctx context.Context, districtCode string) ([]Location, error) {
	return c.locations(ctx, "/districts/"+districtCode+"/wards")
}

func (c *HTTPClient) Rates(ctx context.Context, r RateReq) ([]Rate, error) {
	body := map[string]any{
		"shipment": map[string]any{
			"address_from": map[string]any{"district": r.From.DistrictCode, "city": r.From.CityCode},
			"address_to":   map[string]any{"district": r.To.DistrictCode, "city": r.To.CityCode},
			"parcel": map[string]any{
				"cod":    r.Parcel.CODVND,
				"amount": r.Parcel.AmountVND,
				"weight": r.Parcel.WeightG,
				"length": r.Parcel.LengthCM,
				"width":  r.Parcel.WidthCM,
				"height": r.Parcel.HeightCM,
			},
		},
	}
	var env struct {
		Data []struct {
			ID          string `json:"id"`
			Carrier     string `json:"carrier"`
			CarrierName string `json:"carrier_name"`
			Service     string `json:"service"`
			TotalFee    int64  `json:"total_fee"`
			Expected    string `json:"expected"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodPost, "/rates", body, &env); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRates, err)
	}
	out := make([]Rate, 0, len(env.Data))
	for _, d := range env.Data {
		carrier := d.Carrier
		if carrier == "" {
			carrier = d.CarrierName
		}
		out = append(out, Rate{
			ID: d.ID, Carrier: carrier, CarrierName: d.CarrierName,
			Service: d.Service, FeeVND: d.TotalFee, ETA: d.Expected,
		})
	}
	return out, nil
}

func (c *HTTPClient) CreateShipment(ctx context.Context, r ShipmentReq) (*ShipmentResp, error) {
	body := map[string]any{
		"shipment": map[string]any{
			"rate": r.RateID,
			"address_from": map[string]any{
				"name": r.From.Name, "phone": r.From.Phone, "street": r.From.Street,
				"ward": r.From.WardCode, "district": r.From.DistrictCode, "city": r.From.CityCode,
			},
			"address_to": map[string]any{
				"name": r.To.Name, "phone": r.To.Phone, "street": r.To.Street,
				"ward": r.To.WardCode, "district": r.To.DistrictCode, "city": r.To.CityCode,
			},
			"parcel": map[string]any{
				"cod": r.Parcel.CODVND, "amount": r.Parcel.AmountVND,
				"weight": r.Parcel.WeightG, "length": r.Parcel.LengthCM,
				"width": r.Parcel.WidthCM, "height": r.Parcel.HeightCM,
			},
			"order_id": r.OrderRef,
		},
	}
	var env struct {
		Data struct {
			Code     string `json:"code"`
			GCode    string `json:"gcode"`
			Label    string `json:"label"`
			TotalFee int64  `json:"total_fee"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodPost, "/shipments", body, &env); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCreateShipment, err)
	}
	return &ShipmentResp{
		TrackingCode: env.Data.Code,
		GoshipCode:   env.Data.GCode,
		LabelURL:     env.Data.Label,
		FeeVND:       env.Data.TotalFee,
	}, nil
}

func (c *HTTPClient) VerifyWebhookSignature(rawBody []byte, signature string) error {
	mac := hmac.New(sha256.New, []byte(c.clientSecret))
	mac.Write(rawBody)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return ErrSignatureInvalid
	}
	return nil
}
