package providers

import "strings"

// providerMetadata defines the shared runtime defaults and aliases for a provider.
type providerMetadata struct {
	DefaultAPIBase     string
	EmptyAPIKeyAllowed bool
	Aliases            []string
}

var providerMetadataByName = map[string]providerMetadata{
	"openai": {
		DefaultAPIBase: "https://api.openai.com/v1",
		Aliases:        []string{"gpt"},
	},
	"anthropic": {
		DefaultAPIBase: "https://api.anthropic.com/v1",
		Aliases:        []string{"claude"},
	},
	"anthropic-messages": {
		DefaultAPIBase: "https://api.anthropic.com/v1",
	},
	"gemini": {
		DefaultAPIBase: "https://generativelanguage.googleapis.com/v1beta",
		Aliases:        []string{"google"},
	},
	"deepseek": {
		DefaultAPIBase: "https://api.deepseek.com/v1",
	},
	"openrouter": {
		DefaultAPIBase: "https://openrouter.ai/api/v1",
	},
	"qwen-portal": {
		DefaultAPIBase: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		Aliases:        []string{"qwen"},
	},
	"qwen-intl": {
		DefaultAPIBase: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
		Aliases:        []string{"qwen-international", "dashscope-intl"},
	},
	"moonshot": {
		DefaultAPIBase: "https://api.moonshot.cn/v1",
	},
	"volcengine": {
		DefaultAPIBase: "https://ark.cn-beijing.volces.com/api/v3",
	},
	"zhipu": {
		DefaultAPIBase: "https://open.bigmodel.cn/api/paas/v4",
		Aliases:        []string{"glm"},
	},
	"groq": {
		DefaultAPIBase: "https://api.groq.com/openai/v1",
	},
	"mistral": {
		DefaultAPIBase: "https://api.mistral.ai/v1",
	},
	"nvidia": {
		DefaultAPIBase: "https://integrate.api.nvidia.com/v1",
	},
	"cerebras": {
		DefaultAPIBase: "https://api.cerebras.ai/v1",
	},
	"azure": {
		Aliases: []string{"azure-openai"},
	},
	"bedrock": {
		EmptyAPIKeyAllowed: true,
	},
	"github-copilot": {
		DefaultAPIBase:     "localhost:4321",
		EmptyAPIKeyAllowed: true,
		Aliases:            []string{"copilot"},
	},
	"antigravity": {
		EmptyAPIKeyAllowed: true,
		Aliases:            []string{"google-antigravity"},
	},
	"claude-cli": {
		EmptyAPIKeyAllowed: true,
		Aliases:            []string{"claudecli"},
	},
	"codex-cli": {
		EmptyAPIKeyAllowed: true,
		Aliases:            []string{"codexcli"},
	},
	"ollama": {
		DefaultAPIBase:     "http://localhost:11434/v1",
		EmptyAPIKeyAllowed: true,
	},
	"vllm": {
		DefaultAPIBase:     "http://localhost:8000/v1",
		EmptyAPIKeyAllowed: true,
	},
	"lmstudio": {
		DefaultAPIBase:     "http://localhost:1234/v1",
		EmptyAPIKeyAllowed: true,
	},
	"gpt4free": {
		DefaultAPIBase:     "http://localhost:1337/v1",
		EmptyAPIKeyAllowed: true,
		Aliases:            []string{"g4f"},
	},
	"elevenlabs": {
		DefaultAPIBase: "https://api.elevenlabs.io",
	},
	"venice": {
		DefaultAPIBase: "https://api.venice.ai/api/v1",
	},
	"nearai": {
		DefaultAPIBase: "https://cloud-api.near.ai/v1",
		Aliases:        []string{"near-ai", "near-ai-cloud"},
	},
	"shengsuanyun": {
		DefaultAPIBase: "https://router.shengsuanyun.com/api/v1",
	},
	"siliconflow": {
		DefaultAPIBase: "https://api.siliconflow.cn/v1",
	},
	"vivgrid": {
		DefaultAPIBase: "https://api.vivgrid.com/v1",
	},
	"minimax": {
		DefaultAPIBase: "https://api.minimaxi.com/v1",
	},
	"longcat": {
		DefaultAPIBase: "https://api.longcat.chat/openai",
	},
	"modelscope": {
		DefaultAPIBase: "https://api-inference.modelscope.cn/v1",
	},
	"mimo": {
		DefaultAPIBase: "https://api.xiaomimimo.com/v1",
	},
	"avian": {
		DefaultAPIBase: "https://api.avian.io/v1",
	},
	"zai": {
		DefaultAPIBase: "https://api.z.ai/api/coding/paas/v4",
		Aliases:        []string{"z.ai", "z-ai"},
	},
	"alibaba-coding": {
		DefaultAPIBase: "https://coding-intl.dashscope.aliyuncs.com/v1",
		Aliases:        []string{"coding-plan", "qwen-coding"},
	},
	"alibaba-coding-anthropic": {
		DefaultAPIBase: "https://coding-intl.dashscope.aliyuncs.com/apps/anthropic",
		Aliases:        []string{"coding-plan-anthropic"},
	},
	"novita": {
		DefaultAPIBase: "https://api.novita.ai/openai",
	},
	"litellm": {
		DefaultAPIBase: "http://localhost:4000/v1",
	},
	"qwen-us": {
		DefaultAPIBase: "https://dashscope-us.aliyuncs.com/compatible-mode/v1",
		Aliases:        []string{"dashscope-us"},
	},
}

var normalizedModelProviderAliasesByName = buildModelProviderAliasMap()

func buildModelProviderAliasMap() map[string]string {
	totalAliases := 0
	for _, option := range providerMetadataByName {
		totalAliases += len(option.Aliases)
	}

	aliases := make(map[string]string, len(providerMetadataByName)+totalAliases)
	for provider, option := range providerMetadataByName {
		aliases[provider] = provider
		for _, alias := range option.Aliases {
			normalized := strings.ToLower(strings.TrimSpace(alias))
			if normalized == "" {
				continue
			}
			aliases[normalized] = provider
		}
	}
	return aliases
}

func providerMetadataForName(provider string) (providerMetadata, bool) {
	normalized := NormalizeProvider(provider)
	if normalized == "" {
		return providerMetadata{}, false
	}
	option, ok := providerMetadataByName[normalized]
	return option, ok
}
