package goong

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type HTTPClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewHTTPClient(apiKey, baseURL string) *HTTPClient {
	return &HTTPClient{
		apiKey:     apiKey,
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *HTTPClient) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	q.Set("api_key", c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("goong GET %s: status=%d body=%s", path, resp.StatusCode, string(b))
	}
	return json.Unmarshal(b, out)
}

func latLngStr(p LatLng) string {
	return strconv.FormatFloat(p.Lat, 'f', -1, 64) + "," + strconv.FormatFloat(p.Lng, 'f', -1, 64)
}

func (c *HTTPClient) Geocode(ctx context.Context, query string) ([]GeocodeResult, error) {
	var env struct {
		Results []struct {
			FormattedAddress string `json:"formatted_address"`
			Geometry         struct {
				Location struct {
					Lat float64 `json:"lat"`
					Lng float64 `json:"lng"`
				} `json:"location"`
			} `json:"geometry"`
		} `json:"results"`
	}
	q := url.Values{}
	q.Set("address", query)
	if err := c.getJSON(ctx, "/Geocode", q, &env); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrGeocode, err)
	}
	out := make([]GeocodeResult, 0, len(env.Results))
	for _, r := range env.Results {
		out = append(out, GeocodeResult{
			Lat: r.Geometry.Location.Lat, Lng: r.Geometry.Location.Lng,
			FormattedAddress: r.FormattedAddress,
		})
	}
	return out, nil
}

func (c *HTTPClient) DistanceMatrix(ctx context.Context, origin LatLng, dests []LatLng) ([]DistanceResult, error) {
	parts := make([]string, 0, len(dests))
	for _, d := range dests {
		parts = append(parts, latLngStr(d))
	}
	q := url.Values{}
	q.Set("origins", latLngStr(origin))
	q.Set("destinations", strings.Join(parts, "|"))
	q.Set("vehicle", "car")
	var env struct {
		Rows []struct {
			Elements []struct {
				Distance struct {
					Value int64 `json:"value"`
				} `json:"distance"`
				Duration struct {
					Value int64 `json:"value"`
				} `json:"duration"`
			} `json:"elements"`
		} `json:"rows"`
	}
	if err := c.getJSON(ctx, "/DistanceMatrix", q, &env); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDistance, err)
	}
	if len(env.Rows) == 0 {
		return nil, fmt.Errorf("%w: empty rows", ErrDistance)
	}
	els := env.Rows[0].Elements
	out := make([]DistanceResult, 0, len(els))
	for _, e := range els {
		out = append(out, DistanceResult{DistanceM: e.Distance.Value, DurationS: e.Duration.Value})
	}
	return out, nil
}

func (c *HTTPClient) Directions(ctx context.Context, origin, dest LatLng) (Route, error) {
	q := url.Values{}
	q.Set("origin", latLngStr(origin))
	q.Set("destination", latLngStr(dest))
	q.Set("vehicle", "car")
	var env struct {
		Routes []struct {
			Legs []struct {
				Distance struct {
					Value int64 `json:"value"`
				} `json:"distance"`
				Duration struct {
					Value int64 `json:"value"`
				} `json:"duration"`
			} `json:"legs"`
			OverviewPolyline struct {
				Points string `json:"points"`
			} `json:"overview_polyline"`
		} `json:"routes"`
	}
	if err := c.getJSON(ctx, "/Direction", q, &env); err != nil {
		return Route{}, fmt.Errorf("%w: %v", ErrDirections, err)
	}
	if len(env.Routes) == 0 || len(env.Routes[0].Legs) == 0 {
		return Route{}, fmt.Errorf("%w: no route", ErrDirections)
	}
	r := env.Routes[0]
	return Route{
		DistanceM: r.Legs[0].Distance.Value,
		DurationS: r.Legs[0].Duration.Value,
		Polyline:  r.OverviewPolyline.Points,
	}, nil
}
