package status

import (
	"fmt"
	"os"

	"github.com/sipeed/picoclaw/cmd/picoclaw/internal"
	"github.com/sipeed/picoclaw/cmd/picoclaw/internal/cliui"
	"github.com/sipeed/picoclaw/pkg/auth"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

func statusCmd() {
	cfg, err := internal.LoadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	configPath := internal.GetConfigPath()
	build, _ := config.FormatBuildInfo()

	_, configStatErr := os.Stat(configPath)
	configOK := configStatErr == nil

	workspace := cfg.WorkspacePath()
	_, wsErr := os.Stat(workspace)
	wsOK := wsErr == nil

	report := cliui.StatusReport{
		Logo:          internal.Logo,
		Version:       config.FormatVersion(),
		Build:         build,
		ConfigPath:    configPath,
		ConfigOK:      configOK,
		WorkspacePath: workspace,
		WorkspaceOK:   wsOK,
		Model:         cfg.Agents.Defaults.GetModelName(),
	}

	if configOK {
		// PicoClaw moved to a model-centric configuration (model_list). Status should
		// not depend on a legacy cfg.Providers field (which may not exist under some
		// build tags). We infer provider availability from model_list entries.
		hasProtocolKey := func(protocol string) bool {
			want := providers.NormalizeProvider(protocol)
			for _, m := range cfg.ModelList {
				if m == nil {
					continue
				}
				got, _ := providers.ExtractProtocol(m)
				if got == want && m.APIKey() != "" {
					return true
				}
			}
			return false
		}
		val := func(enabled bool) string {
			if enabled {
				return "✓"
			}
			return "not set"
		}

		report.Providers = []cliui.ProviderRow{
			{Name: "OpenAI API", Val: val(hasProtocolKey("openai"))},
			{Name: "Anthropic API", Val: val(hasProtocolKey("anthropic"))},
		}

		store, _ := auth.LoadStore()
		if store != nil && len(store.Credentials) > 0 {
			for provider, cred := range store.Credentials {
				protocol := providers.NormalizeProvider(provider)
				if protocol != "openai" && protocol != "anthropic" {
					continue
				}
				st := "authenticated"
				if cred.IsExpired() {
					st = "expired"
				} else if cred.NeedsRefresh() {
					st = "needs refresh"
				}
				report.OAuthLines = append(report.OAuthLines,
					fmt.Sprintf("%s (%s): %s", provider, cred.AuthMethod, st))
			}
		}
	}

	cliui.PrintStatus(report)
}
