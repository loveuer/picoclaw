package audio

import (
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// ResolveModel keeps speech-only services independent of the chat provider catalog.
func ResolveModel(cfg *config.ModelConfig) (protocol, modelID, apiBase string) {
	if cfg == nil {
		return "", "", ""
	}
	defaults := map[string]string{
		"elevenlabs": "https://api.elevenlabs.io",
		"groq":       "https://api.groq.com/openai/v1",
		"mimo":       "https://api.xiaomimimo.com/v1",
	}
	protocol = strings.ToLower(strings.TrimSpace(cfg.Provider))
	modelID = strings.TrimSpace(cfg.Model)
	if protocol == "" {
		prefix, rest, found := strings.Cut(modelID, "/")
		if _, ok := defaults[strings.ToLower(prefix)]; found && ok {
			protocol, modelID = strings.ToLower(prefix), rest
		} else {
			protocol, modelID = providers.ExtractProtocol(cfg)
		}
	}
	apiBase = strings.TrimRight(strings.TrimSpace(cfg.APIBase), "/")
	if apiBase == "" {
		apiBase = defaults[protocol]
		if apiBase == "" {
			apiBase = providers.DefaultAPIBaseForProtocol(protocol)
		}
	}
	return protocol, modelID, apiBase
}
