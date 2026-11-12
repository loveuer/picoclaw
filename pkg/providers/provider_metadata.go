package providers

import "strings"

// providerMetadata defines the shared runtime defaults and aliases for a provider.
type providerMetadata struct {
	DefaultAPIBase string
	Aliases        []string
}

var providerMetadataByName = map[string]providerMetadata{
	"openai":    {DefaultAPIBase: "https://api.openai.com/v1", Aliases: []string{"gpt"}},
	"anthropic": {DefaultAPIBase: "https://api.anthropic.com/v1", Aliases: []string{"claude", "anthropic-messages"}},
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
