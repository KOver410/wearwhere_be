package llm

import "context"

// MockClient returns a deterministic canned response. Used for dev/test/CI so
// no network or API key is needed (AI_PROVIDER=mock, the default).
type MockClient struct {
	Response string
}

func NewMockClient() *MockClient { return &MockClient{} }

// DefaultMockOutfitJSON is a canned outfit grouping referencing item ids "1","2".
const DefaultMockOutfitJSON = `{"outfits":[{"title":"Everyday look","note":"A simple, versatile pairing.","item_ids":["1","2"]}]}`

func (m *MockClient) Generate(_ context.Context, _ GenerateRequest) (*GenerateResponse, error) {
	text := m.Response
	if text == "" {
		text = DefaultMockOutfitJSON
	}
	return &GenerateResponse{Text: text, TokensIn: 0, TokensOut: 0, Model: "mock"}, nil
}
