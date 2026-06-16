package llm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/shared/llm"
)

func TestGeminiClient_GenerateMapsResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.URL.Path, ":generateContent")
		require.Equal(t, "test-key", r.URL.Query().Get("key"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates":[{"content":{"parts":[{"text":"{\"outfits\":[]}"}]},"finishReason":"STOP"}],
			"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":7}
		}`))
	}))
	defer ts.Close()

	c := llm.NewGeminiClient("test-key", "gemini-2.0-flash", ts.URL, 5*time.Second)
	resp, err := c.Generate(context.Background(), llm.GenerateRequest{System: "sys", Prompt: "hi"})
	require.NoError(t, err)
	require.Equal(t, `{"outfits":[]}`, resp.Text)
	require.Equal(t, 12, resp.TokensIn)
	require.Equal(t, 7, resp.TokensOut)
	require.Equal(t, "gemini-2.0-flash", resp.Model)
}

func TestGeminiClient_ErrorOnNon2xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"bad"}`, http.StatusInternalServerError)
	}))
	defer ts.Close()
	c := llm.NewGeminiClient("k", "m", ts.URL, 5*time.Second)
	_, err := c.Generate(context.Background(), llm.GenerateRequest{Prompt: "x"})
	require.ErrorIs(t, err, llm.ErrUnavailable)
}

func TestGeminiClient_ErrorOnEmptyCandidates(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"candidates":[]}`))
	}))
	defer ts.Close()
	c := llm.NewGeminiClient("k", "m", ts.URL, 5*time.Second)
	_, err := c.Generate(context.Background(), llm.GenerateRequest{Prompt: "x"})
	require.ErrorIs(t, err, llm.ErrUnavailable)
	require.True(t, strings.Contains(err.Error(), "empty candidates"))
}
