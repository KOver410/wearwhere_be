package llm_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/shared/llm"
)

func TestMockClient_DefaultAndOverride(t *testing.T) {
	m := llm.NewMockClient()
	resp, err := m.Generate(context.Background(), llm.GenerateRequest{Prompt: "x"})
	require.NoError(t, err)
	require.Equal(t, llm.DefaultMockOutfitJSON, resp.Text)
	require.Equal(t, "mock", resp.Model)

	m.Response = `{"outfits":[]}`
	resp, err = m.Generate(context.Background(), llm.GenerateRequest{Prompt: "x"})
	require.NoError(t, err)
	require.Equal(t, `{"outfits":[]}`, resp.Text)
}

func TestNewFromConfig(t *testing.T) {
	c, err := llm.NewFromConfig(llm.Config{Provider: "mock"})
	require.NoError(t, err)
	require.NotNil(t, c)

	_, err = llm.NewFromConfig(llm.Config{Provider: "gemini"})
	require.Error(t, err)

	_, err = llm.NewFromConfig(llm.Config{Provider: "bogus"})
	require.Error(t, err)
}
