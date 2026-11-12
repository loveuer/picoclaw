package providers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestFactoryRejectsRemovedProtocols(t *testing.T) {
	for _, protocol := range []string{"gemini", "azure", "bedrock", "openrouter", "deepseek", "ollama", "antigravity", "github-copilot", "claude-cli", "codex-cli"} {
		t.Run(protocol, func(t *testing.T) {
			cfg := &config.ModelConfig{
				Provider: protocol,
				Model:    "remote-model",
				APIBase:  "http://127.0.0.1:1/v1",
				APIKeys:  config.SimpleSecureStrings("test-key"),
			}
			_, _, err := CreateProviderFromConfig(cfg)
			require.ErrorContains(t, err, "use openai or anthropic")
			require.False(t, IsSupportedModelProvider(protocol))
		})
	}
}

func TestDefaultModelsUseSupportedProtocols(t *testing.T) {
	models := config.DefaultConfig().ModelList
	protocols := make(map[string]bool)
	for _, model := range models {
		protocol, _ := ExtractProtocol(model)
		require.Contains(t, []string{"openai", "anthropic"}, protocol)
		protocols[protocol] = true
	}
	require.Len(t, protocols, 2)
}

func TestFactoryAnthropicUsesNativeMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/messages", r.URL.Path)
		require.Equal(t, "test-key", r.Header.Get("X-Api-Key"))
		require.Empty(t, r.Header.Get("Authorization"))
		require.NotEmpty(t, r.Header.Get("Anthropic-Version"))
		require.Equal(t, "fork-test", r.Header.Get("User-Agent"))
		require.Equal(t, "custom", r.Header.Get("X-Test"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "claude-sonnet-4-6", body["model"])
		require.Equal(t, "value", body["custom_option"])
		require.Contains(t, body, "thinking")
		require.Contains(t, body, "system")
		require.Len(t, body["tools"], 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(
			[]byte(
				`{"id":"msg_test","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"tool_use","id":"call_test","name":"weather","input":{"city":"成都"}}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`,
			),
		)
	}))
	defer server.Close()

	cfg := &config.ModelConfig{
		Provider:       "anthropic",
		Model:          "claude-sonnet-4-6",
		APIBase:        server.URL + "/v1",
		APIKeys:        config.SimpleSecureStrings("test-key"),
		RequestTimeout: 5,
		UserAgent:      "fork-test",
		ExtraBody:      map[string]any{"custom_option": "value"},
		CustomHeaders:  map[string]string{"X-Test": "custom"},
	}
	provider, model, err := CreateProviderFromConfig(cfg)
	require.NoError(t, err)
	response, err := provider.Chat(t.Context(), []Message{
		{Role: "system", Content: "Help with weather."},
		{Role: "user", Content: "成都天气"},
	}, []ToolDefinition{{Type: "function", Function: ToolFunctionDefinition{
		Name: "weather",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"city": map[string]any{"type": "string"},
			},
		},
	}}}, model, map[string]any{"max_tokens": 8192, "thinking_level": "high"})
	require.NoError(t, err)
	require.Len(t, response.ToolCalls, 1)
	require.Equal(t, "weather", response.ToolCalls[0].Name)
	require.Equal(t, "成都", response.ToolCalls[0].Arguments["city"])
}
