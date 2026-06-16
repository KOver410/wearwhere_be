// Package llm is a provider-agnostic text-generation port. Adapters: gemini
// (HTTP) and mock (deterministic). Select via factory.NewFromConfig.
package llm

import (
	"context"
	"errors"
)

// ErrUnavailable means the provider failed (timeout, non-2xx, decode error,
// or safety block). Callers degrade gracefully rather than surfacing 500s.
var ErrUnavailable = errors.New("llm: provider unavailable")

// GenerateRequest is a single-shot generation: an optional system instruction
// plus the user prompt.
type GenerateRequest struct {
	System string
	Prompt string
}

// GenerateResponse is the model output plus token accounting.
type GenerateResponse struct {
	Text      string
	TokensIn  int
	TokensOut int
	Model     string
}

// Client generates text from a prompt.
type Client interface {
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
}
