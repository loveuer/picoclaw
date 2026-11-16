package status

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/auth"
	"github.com/sipeed/picoclaw/pkg/config"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = w

	fn()

	_ = w.Close()
	os.Stdout = oldStdout
	defer r.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy() error = %v", err)
	}
	return buf.String()
}

func TestStatusCmd_RecognizesProviderFieldWithoutModelPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	workspace := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	t.Setenv(config.EnvConfig, configPath)
	t.Setenv(config.EnvHome, tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				ModelName:   "gpt-5.4",
				Workspace:   workspace,
				Provider:    "openai",
				MaxTokens:   65536,
				Temperature: nil,
			},
		},
		ModelList: []*config.ModelConfig{
			{
				ModelName: "gpt-5.4",
				Provider:  "openai",
				Model:     "gpt-5.4",
				APIBase:   "https://api.openai.com/v1",
				APIKeys:   config.SimpleSecureStrings("test-key"),
				Enabled:   true,
			},
			{
				ModelName: "claude-sonnet-4.6",
				Provider:  "anthropic",
				Model:     "claude-sonnet-4.6",
				APIBase:   "https://api.anthropic.com/v1",
				APIKeys:   config.SimpleSecureStrings("test-key"),
				Enabled:   true,
			},
		},
	}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("config.SaveConfig() error = %v", err)
	}

	if err := auth.SaveStore(&auth.AuthStore{Credentials: map[string]*auth.AuthCredential{
		"openai":    {AuthMethod: "oauth"},
		"anthropic": {AuthMethod: "token"},
		"gemini":    {AuthMethod: "oauth"},
	}}); err != nil {
		t.Fatalf("auth.SaveStore() error = %v", err)
	}

	output := captureStdout(t, statusCmd)

	if !strings.Contains(output, "OpenAI API: \u2713") {
		t.Fatalf("status output missing OpenAI provider: %s", output)
	}
	if !strings.Contains(output, "Anthropic API: \u2713") {
		t.Fatalf("status output missing Anthropic provider: %s", output)
	}
	for _, removed := range []string{
		"OpenRouter API", "Gemini API", "Zhipu API", "Qwen API", "Groq API",
		"Moonshot API", "DeepSeek API", "VolcEngine API", "Nvidia API", "vLLM / local", "Ollama",
	} {
		if strings.Contains(output, removed) {
			t.Errorf("status output includes removed provider %q: %s", removed, output)
		}
	}
	for _, expected := range []string{"openai (oauth): authenticated", "anthropic (token): authenticated"} {
		if !strings.Contains(output, expected) {
			t.Errorf("status output missing authentication state %q: %s", expected, output)
		}
	}
	if strings.Contains(output, "gemini") {
		t.Errorf("status output includes removed authentication provider: %s", output)
	}
}
