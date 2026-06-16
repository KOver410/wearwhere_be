package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GeminiClient calls the Generative Language API generateContent endpoint.
type GeminiClient struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

// NewGeminiClient builds the adapter. baseURL is configurable so tests can
// point it at an httptest server.
func NewGeminiClient(apiKey, model, baseURL string, timeout time.Duration) *GeminiClient {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &GeminiClient{apiKey: apiKey, model: model, baseURL: baseURL, httpClient: &http.Client{Timeout: timeout}}
}

type geminiPart struct {
	Text string `json:"text"`
}
type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}
type geminiGenCfg struct {
	ResponseMIMEType string `json:"responseMimeType,omitempty"`
}
type geminiRequest struct {
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	Contents          []geminiContent `json:"contents"`
	GenerationConfig  *geminiGenCfg   `json:"generationConfig,omitempty"`
}
type geminiResponse struct {
	Candidates []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
}

func (c *GeminiClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	body := geminiRequest{
		Contents:         []geminiContent{{Role: "user", Parts: []geminiPart{{Text: req.Prompt}}}},
		GenerationConfig: &geminiGenCfg{ResponseMIMEType: "application/json"},
	}
	if req.System != "" {
		body.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: req.System}}}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal: %v", ErrUnavailable, err)
	}
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", c.baseURL, c.model, c.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: new request: %v", ErrUnavailable, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: status=%d body=%s", ErrUnavailable, resp.StatusCode, string(b))
	}
	var gr geminiResponse
	if err := json.Unmarshal(b, &gr); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrUnavailable, err)
	}
	if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("%w: empty candidates (finish reason may be safety)", ErrUnavailable)
	}
	return &GenerateResponse{
		Text:      gr.Candidates[0].Content.Parts[0].Text,
		TokensIn:  gr.UsageMetadata.PromptTokenCount,
		TokensOut: gr.UsageMetadata.CandidatesTokenCount,
		Model:     c.model,
	}, nil
}
