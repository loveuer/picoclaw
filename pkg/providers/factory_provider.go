// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package providers

import (
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	anthropicprovider "github.com/sipeed/picoclaw/pkg/providers/anthropic"
)

// createClaudeAuthProvider creates a Claude provider using OAuth credentials from auth store.
func createClaudeAuthProvider() (LLMProvider, error) {
	cred, err := getCredential("anthropic")
	if err != nil {
		return nil, fmt.Errorf("loading auth credentials: %w", err)
	}
	if cred == nil {
		return nil, fmt.Errorf("no credentials for anthropic. Run: picoclaw auth login --provider anthropic")
	}
	return NewClaudeProviderWithTokenSource(cred.AccessToken, createClaudeTokenSource()), nil
}

// createCodexAuthProvider creates a Codex provider using OAuth credentials from auth store.
func createCodexAuthProvider() (LLMProvider, error) {
	cred, err := getCredential("openai")
	if err != nil {
		return nil, fmt.Errorf("loading auth credentials: %w", err)
	}
	if cred == nil {
		return nil, fmt.Errorf("no credentials for openai. Run: picoclaw auth login --provider openai")
	}
	return NewCodexProviderWithTokenSource(cred.AccessToken, cred.AccountID, createCodexTokenSource()), nil
}

// ExtractProtocol extracts the effective protocol and model identifier from a
// model configuration.
//
// The explicit Provider field takes precedence. When Provider is empty, the
// protocol is inferred from Model. Plain model names default to "openai".
// Provider-prefixed models strip the first slash-separated segment from the
// returned model ID.
//
// The returned protocol is normalized to the provider's canonical spelling.
// Examples:
//   - Model "openai/gpt-4o" -> ("openai", "gpt-4o")
//   - Model "openai/vendor/model" -> ("openai", "vendor/model")
//   - Provider "openai", Model "vendor/model" -> ("openai", "vendor/model")
//   - Provider "openai", Model "openai/gpt-4o" -> ("openai", "openai/gpt-4o")
//   - Model "gpt-4o" -> ("openai", "gpt-4o")
func ExtractProtocol(cfg *config.ModelConfig) (protocol, modelID string) {
	if cfg == nil {
		return "", ""
	}

	model := strings.TrimSpace(cfg.Model)
	if provider := strings.TrimSpace(cfg.Provider); provider != "" {
		return NormalizeProvider(provider), model
	}
	return SplitModelProviderAndID(model, "openai")
}

// ResolveAPIBase returns the configured API base, or the protocol default when
// the model uses an HTTP-based provider family with a known default endpoint.
func ResolveAPIBase(cfg *config.ModelConfig) string {
	if cfg == nil {
		return ""
	}
	if apiBase := strings.TrimSpace(cfg.APIBase); apiBase != "" {
		return strings.TrimRight(apiBase, "/")
	}
	protocol, _ := ExtractProtocol(cfg)
	return strings.TrimRight(getDefaultAPIBase(protocol), "/")
}

// CreateProviderFromConfig creates a provider based on the ModelConfig.
// It uses ExtractProtocol to determine which provider to create.
// Supported protocols are OpenAI (Responses and Chat Completions) and Anthropic Messages.
// See the switch on protocol in this function for the authoritative list.
// Returns the provider, the effective model ID from ExtractProtocol, and any error.
func CreateProviderFromConfig(cfg *config.ModelConfig) (LLMProvider, string, error) {
	if cfg == nil {
		return nil, "", fmt.Errorf("config is nil")
	}

	if cfg.Model == "" {
		return nil, "", fmt.Errorf("model is required")
	}

	protocol, modelID := ExtractProtocol(cfg)
	authMethod := strings.ToLower(strings.TrimSpace(cfg.AuthMethod))

	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = fmt.Sprintf("PicoClaw/%s", config.Version)
	}

	switch protocol {
	case "openai":
		// OpenAI with OAuth/token auth (Codex-style)
		if authMethod == "oauth" || authMethod == "token" {
			provider, err := createCodexAuthProvider()
			if err != nil {
				return nil, "", err
			}
			return finalizeProviderFromConfig(provider, modelID, cfg)
		}
		// OpenAI with API key
		if cfg.APIKey() == "" && cfg.APIBase == "" {
			return nil, "", fmt.Errorf("api_key or api_base is required for HTTP-based protocol %q", protocol)
		}
		apiBase := cfg.APIBase
		if apiBase == "" {
			apiBase = getDefaultAPIBase(protocol)
		}
		provider := NewHTTPProviderWithMaxTokensFieldAndRequestTimeout(
			cfg.APIKey(),
			apiBase,
			cfg.Proxy,
			cfg.MaxTokensField,
			userAgent,
			cfg.RequestTimeout,
			cfg.ExtraBody,
			cfg.CustomHeaders,
		)
		provider.SetProviderName(protocol)
		return finalizeProviderFromConfig(provider, modelID, cfg)

	case "anthropic":
		if authMethod == "oauth" || authMethod == "token" {
			// Use OAuth credentials from auth store
			provider, err := createClaudeAuthProvider()
			if err != nil {
				return nil, "", err
			}
			return finalizeProviderFromConfig(provider, modelID, cfg)
		}
		if cfg.APIKey() == "" {
			return nil, "", fmt.Errorf("api_key is required for anthropic protocol (model: %s)", cfg.Model)
		}
		provider := anthropicprovider.NewAPIKeyProvider(
			cfg.APIKey(),
			cfg.APIBase,
			cfg.Proxy,
			userAgent,
			cfg.RequestTimeout,
			cfg.ExtraBody,
			cfg.CustomHeaders,
		)
		return finalizeProviderFromConfig(provider, modelID, cfg)

	default:
		return nil, "", fmt.Errorf(
			"unsupported model protocol %q in model %q; use openai or anthropic with an explicit api_base for compatible services",
			protocol,
			cfg.Model,
		)
	}
}

func finalizeProviderFromConfig(
	provider LLMProvider,
	modelID string,
	cfg *config.ModelConfig,
) (LLMProvider, string, error) {
	wrapped, err := wrapProviderWithToolSchemaTransform(provider, cfg.ToolSchemaTransform)
	if err != nil {
		return nil, "", err
	}
	return wrapped, modelID, nil
}

// DefaultAPIBaseForProtocol returns the default endpoint for a supported protocol.
func DefaultAPIBaseForProtocol(protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	return getDefaultAPIBase(protocol)
}

// getDefaultAPIBase returns the default API base URL for a given protocol.
func getDefaultAPIBase(protocol string) string {
	option, ok := providerMetadataForName(protocol)
	if !ok {
		return ""
	}
	return option.DefaultAPIBase
}
