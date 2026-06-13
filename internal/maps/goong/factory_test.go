package goong

import "testing"

func TestNewFromConfig_MockByDefault(t *testing.T) {
	c, err := NewFromConfig(Config{Mode: "mock"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := c.(*MockClient); !ok {
		t.Fatalf("expected *MockClient, got %T", c)
	}
}

func TestNewFromConfig_ProductionRequiresKey(t *testing.T) {
	if _, err := NewFromConfig(Config{Mode: "production"}); err == nil {
		t.Fatal("expected error when production mode has no API key")
	}
}

func TestNewFromConfig_ProductionWithKey(t *testing.T) {
	c, err := NewFromConfig(Config{Mode: "production", APIKey: "k", BaseURL: "https://rsapi.goong.io"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := c.(*HTTPClient); !ok {
		t.Fatalf("expected *HTTPClient, got %T", c)
	}
}
