// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package providers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/auth"
	"github.com/sipeed/picoclaw/pkg/config"
)

func TestExtractProtocol(t *testing.T) {
	tests := []struct {
		name         string
		config       *config.ModelConfig
		wantProtocol string
		wantModelID  string
	}{
		{
			name:         "openai with prefix",
			config:       &config.ModelConfig{Model: "openai/gpt-4o"},
			wantProtocol: "openai",
			wantModelID:  "gpt-4o",
		},
		{
			name:         "anthropic with prefix",
			config:       &config.ModelConfig{Model: "anthropic/claude-sonnet-4.6"},
			wantProtocol: "anthropic",
			wantModelID:  "claude-sonnet-4.6",
		},
		{
			name:         "no prefix - defaults to openai",
			config:       &config.ModelConfig{Model: "gpt-4o"},
			wantProtocol: "openai",
			wantModelID:  "gpt-4o",
		},
		{
			name:         "empty string",
			config:       &config.ModelConfig{Model: ""},
			wantProtocol: "",
			wantModelID:  "",
		},
		{
			name:         "with whitespace",
			config:       &config.ModelConfig{Model: "  openai/gpt-4  "},
			wantProtocol: "openai",
			wantModelID:  "gpt-4",
		},
		{
			name:         "multiple slashes",
			config:       &config.ModelConfig{Model: "openai/meta/llama-3.1-8b"},
			wantProtocol: "openai",
			wantModelID:  "meta/llama-3.1-8b",
		},
		{
			name:         "normalizes provider",
			config:       &config.ModelConfig{Model: "gpt/glm-5.1"},
			wantProtocol: "openai",
			wantModelID:  "glm-5.1",
		},
		{
			name:         "explicit provider keeps model",
			config:       &config.ModelConfig{Provider: "openai", Model: "z-ai/glm-5.1"},
			wantProtocol: "openai",
			wantModelID:  "z-ai/glm-5.1",
		},
		{
			name:         "explicit provider preserves matching prefix",
			config:       &config.ModelConfig{Provider: "openai", Model: "openai/gpt-4o"},
			wantProtocol: "openai",
			wantModelID:  "openai/gpt-4o",
		},
		{
			name:         "explicit provider preserves aliased prefix",
			config:       &config.ModelConfig{Provider: "gpt", Model: "qwen/qwen-plus"},
			wantProtocol: "openai",
			wantModelID:  "qwen/qwen-plus",
		},
		{
			name:         "empty provider segment",
			config:       &config.ModelConfig{Model: "/gpt-4o"},
			wantProtocol: "",
			wantModelID:  "gpt-4o",
		},
		{
			name:         "unknown prefix falls back to openai",
			config:       &config.ModelConfig{Model: "meta-llama/Llama-3.1-8B-Instruct"},
			wantProtocol: "openai",
			wantModelID:  "meta-llama/Llama-3.1-8B-Instruct",
		},
		{
			name:         "nil config",
			wantProtocol: "",
			wantModelID:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protocol, modelID := ExtractProtocol(tt.config)
			if protocol != tt.wantProtocol {
				t.Errorf("ExtractProtocol() protocol = %q, want %q", protocol, tt.wantProtocol)
			}
			if modelID != tt.wantModelID {
				t.Errorf("ExtractProtocol() modelID = %q, want %q", modelID, tt.wantModelID)
			}
		})
	}
}

func TestCreateProviderFromConfig_OpenAI(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "test-openai",
		Model:     "openai/gpt-4o",
		APIBase:   "https://api.example.com/v1",
	}
	cfg.SetAPIKey("test-key")

	provider, modelID, err := CreateProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}
	if provider == nil {
		t.Fatal("CreateProviderFromConfig() returned nil provider")
	}
	if modelID != "gpt-4o" {
		t.Errorf("modelID = %q, want %q", modelID, "gpt-4o")
	}
}

func TestCreateProviderFromConfig_UsesExplicitProvider(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "test-explicit-provider",
		Model:     "z-ai/glm-5.1",
		Provider:  "openai",
	}
	cfg.SetAPIKey("test-key")

	provider, modelID, err := CreateProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}
	if provider == nil {
		t.Fatal("CreateProviderFromConfig() returned nil provider")
	}
	if modelID != "z-ai/glm-5.1" {
		t.Fatalf("modelID = %q, want z-ai/glm-5.1", modelID)
	}
	if got := ResolveAPIBase(cfg); got != "https://api.openai.com/v1" {
		t.Fatalf("ResolveAPIBase() = %q, want OpenAI default API base", got)
	}
}

func TestCreateProviderFromConfig_PreservesExplicitProviderPrefixedModel(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "test-openai",
		Provider:  "openai",
		Model:     "openai/gpt-4o",
		APIBase:   "https://api.example.com/v1",
	}
	cfg.SetAPIKey("test-key")

	provider, modelID, err := CreateProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}
	if provider == nil {
		t.Fatal("CreateProviderFromConfig() returned nil provider")
	}
	if modelID != "openai/gpt-4o" {
		t.Fatalf("modelID = %q, want %q", modelID, "openai/gpt-4o")
	}
}

func TestCreateProviderFromConfig_Anthropic(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "test-anthropic",
		Model:     "anthropic/claude-sonnet-4.6",
	}
	cfg.SetAPIKey("test-key")

	provider, modelID, err := CreateProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}
	if provider == nil {
		t.Fatal("CreateProviderFromConfig() returned nil provider")
	}
	if modelID != "claude-sonnet-4.6" {
		t.Errorf("modelID = %q, want %q", modelID, "claude-sonnet-4.6")
	}
}

func TestCreateProviderFromConfig_OpenAIMixedCaseAuthMethodUsesOAuthBranch(t *testing.T) {
	origGetCredential := getCredential
	getCredential = func(provider string) (*auth.AuthCredential, error) {
		if provider != "openai" {
			t.Fatalf("provider = %q, want %q", provider, "openai")
		}
		return &auth.AuthCredential{
			AccessToken: "test-token",
			AccountID:   "acct-test",
			Provider:    "openai",
			AuthMethod:  "oauth",
		}, nil
	}
	t.Cleanup(func() {
		getCredential = origGetCredential
	})

	cfg := &config.ModelConfig{
		ModelName:  "test-openai-oauth",
		Model:      "openai/gpt-5.4",
		AuthMethod: "OAuth",
	}

	provider, modelID, err := CreateProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}
	if provider == nil {
		t.Fatal("CreateProviderFromConfig() returned nil provider")
	}
	if modelID != "gpt-5.4" {
		t.Errorf("modelID = %q, want %q", modelID, "gpt-5.4")
	}
}

func TestCreateProviderFromConfig_MissingAPIKey(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "test-no-key",
		Model:     "openai/gpt-4o",
	}

	_, _, err := CreateProviderFromConfig(cfg)
	if err == nil {
		t.Fatal("CreateProviderFromConfig() expected error for missing API key")
	}
}

func TestCreateProviderFromConfig_UnknownProtocol(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "test-unknown-provider",
		Provider:  "unknown-protocol",
		Model:     "model",
	}
	cfg.SetAPIKey("test-key")

	_, _, err := CreateProviderFromConfig(cfg)
	if err == nil {
		t.Fatal("CreateProviderFromConfig() expected error for unknown protocol")
	}
}

func TestCreateProviderFromConfig_UnknownModelPrefixDefaultsToOpenAI(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "test-unknown-model-prefix",
		Model:     "meta-llama/Llama-3.1-8B-Instruct",
		APIBase:   "https://api.example.com/v1",
	}
	cfg.SetAPIKey("test-key")

	provider, modelID, err := CreateProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}
	if provider == nil {
		t.Fatal("CreateProviderFromConfig() returned nil provider")
	}
	if modelID != "meta-llama/Llama-3.1-8B-Instruct" {
		t.Fatalf("modelID = %q, want full model ID", modelID)
	}
}

func TestCreateProviderFromConfig_NilConfig(t *testing.T) {
	_, _, err := CreateProviderFromConfig(nil)
	if err == nil {
		t.Fatal("CreateProviderFromConfig(nil) expected error")
	}
}

func TestCreateProviderFromConfig_EmptyModel(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "test-empty",
		Model:     "",
	}

	_, _, err := CreateProviderFromConfig(cfg)
	if err == nil {
		t.Fatal("CreateProviderFromConfig() expected error for empty model")
	}
}

func TestCreateProviderFromConfig_RequestTimeoutPropagation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1500 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	cfg := &config.ModelConfig{
		ModelName:      "test-timeout",
		Model:          "openai/gpt-4o",
		APIBase:        server.URL,
		RequestTimeout: 1,
	}
	cfg.SetAPIKey("test-key")

	provider, modelID, err := CreateProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}
	if modelID != "gpt-4o" {
		t.Fatalf("modelID = %q, want %q", modelID, "gpt-4o")
	}

	_, err = provider.Chat(
		t.Context(),
		[]Message{{Role: "user", Content: "hi"}},
		nil,
		modelID,
		nil,
	)
	if err == nil {
		t.Fatal("Chat() expected timeout error, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "context deadline exceeded") && !strings.Contains(errMsg, "Client.Timeout exceeded") {
		t.Fatalf("Chat() error = %q, want timeout-related error", errMsg)
	}
}

func TestCreateProviderFromConfig_CustomHeaders(t *testing.T) {
	var gotSource, gotAuth, gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSource = r.Header.Get("X-Source")
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(openaiResponsesResponse))
	}))
	defer server.Close()

	cfg := &config.ModelConfig{
		ModelName:     "test-headers",
		Model:         "openai/gpt-4o",
		APIBase:       server.URL,
		CustomHeaders: map[string]string{"X-Source": "coding-plan", "Authorization": "Token config-auth"},
	}
	cfg.SetAPIKey("test-key")

	provider, modelID, err := CreateProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}

	_, err = provider.Chat(
		t.Context(),
		[]Message{{Role: "user", Content: "hi"}},
		nil,
		modelID,
		nil,
	)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if gotSource != "coding-plan" {
		t.Fatalf("X-Source = %q, want %q", gotSource, "coding-plan")
	}
	if gotAuth != "Token config-auth" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Token config-auth")
	}
	if gotPath != "/responses" {
		t.Fatalf("request path = %q, want /responses", gotPath)
	}
}

// openaiCompatResponse is the JSON response used by OpenAI-compatible providers.
const openaiCompatResponse = `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`

// openaiResponsesResponse is the JSON response used by native Responses API providers.
const openaiResponsesResponse = `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`

// anthropicResponse is the JSON response used by Anthropic providers.
const anthropicResponse = `{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","model":"claude-sonnet-4-20250514","usage":{"input_tokens":10,"output_tokens":5}}`

func TestCreateProviderFromConfig_UserAgent(t *testing.T) {
	defaultUA := "PicoClaw/" + config.Version

	tests := []struct {
		name      string
		model     string
		userAgent string
		apiKey    string
		response  string
		wantUA    string
		chatOpts  map[string]any
	}{
		{
			name:     "openai default user agent",
			model:    "openai/gpt-4o",
			apiKey:   "test-key",
			response: openaiResponsesResponse,
			wantUA:   defaultUA,
		},
		{
			name:      "openai custom user agent",
			model:     "openai/gpt-4o",
			apiKey:    "test-key",
			userAgent: "MyAgent/1.2.3",
			response:  openaiResponsesResponse,
			wantUA:    "MyAgent/1.2.3",
		},
		{
			name:     "anthropic default user agent",
			model:    "anthropic/claude-sonnet-4-20250514",
			apiKey:   "test-key",
			response: anthropicResponse,
			wantUA:   defaultUA,
		},
		{
			name:     "anthropic-messages default user agent",
			model:    "anthropic-messages/claude-sonnet-4-20250514",
			apiKey:   "test-key",
			response: anthropicResponse,
			wantUA:   defaultUA,
			chatOpts: map[string]any{"max_tokens": 1024},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedUA string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedUA = r.Header.Get("User-Agent")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()

			cfg := &config.ModelConfig{
				ModelName: "test-ua-" + tt.name,
				Model:     tt.model,
				APIBase:   server.URL,
				UserAgent: tt.userAgent,
			}
			cfg.SetAPIKey(tt.apiKey)

			provider, modelID, err := CreateProviderFromConfig(cfg)
			if err != nil {
				t.Fatalf("CreateProviderFromConfig() error = %v", err)
			}
			if provider == nil {
				t.Fatal("CreateProviderFromConfig() returned nil provider")
			}

			_, err = provider.Chat(
				t.Context(),
				[]Message{{Role: "user", Content: "hi"}},
				nil,
				modelID,
				tt.chatOpts,
			)
			if err != nil {
				t.Fatalf("Chat() error = %v", err)
			}

			if receivedUA != tt.wantUA {
				t.Errorf("User-Agent = %q, want %q", receivedUA, tt.wantUA)
			}
		})
	}
}

func TestCreateProviderFromConfig_ToolSchemaTransformWrapsProvider(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName:           "anthropic-test",
		Provider:            "anthropic",
		Model:               "claude-sonnet-4.6",
		APIKeys:             config.SimpleSecureStrings("test-key"),
		ToolSchemaTransform: "simple",
	}

	provider, modelID, err := CreateProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}
	if modelID != "claude-sonnet-4.6" {
		t.Fatalf("modelID = %q, want %q", modelID, "claude-sonnet-4.6")
	}
	if _, ok := provider.(*toolSchemaStreamingProvider); !ok {
		t.Fatalf("provider = %T, want *toolSchemaStreamingProvider", provider)
	}
}

func TestCreateProviderFromConfig_InvalidToolSchemaTransform(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName:           "anthropic-test",
		Provider:            "anthropic",
		Model:               "claude-sonnet-4.6",
		APIKeys:             config.SimpleSecureStrings("test-key"),
		ToolSchemaTransform: "invalid",
	}

	_, _, err := CreateProviderFromConfig(cfg)
	if err == nil {
		t.Fatal("CreateProviderFromConfig() expected error for invalid tool_schema_transform")
	}
	if !strings.Contains(err.Error(), "tool_schema_transform") {
		t.Fatalf("error = %v, want mention tool_schema_transform", err)
	}
}
