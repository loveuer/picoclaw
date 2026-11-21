package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/cmd/picoclaw/internal"
	"github.com/sipeed/picoclaw/pkg/channels/weixin"
	"github.com/sipeed/picoclaw/pkg/config"
)

func newWeixinCommand() *cobra.Command {
	var baseURL string
	var proxy string
	var timeout int

	cmd := &cobra.Command{
		Use:   "weixin",
		Short: "Connect a WeChat personal account via QR code",
		Long: `Start the interactive Weixin (WeChat personal) QR code login flow.

A QR code is displayed in the terminal. Scan it with the WeChat mobile app
to authorize your account. On success, the bot token is saved to the picoclaw
config so you can start the gateway immediately.

Example:
  picoclaw auth weixin`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWeixinOnboard(baseURL, proxy, time.Duration(timeout)*time.Second)
		},
	}

	cmd.Flags().StringVar(&baseURL, "base-url", "https://ilinkai.weixin.qq.com/", "iLink API base URL")
	cmd.Flags().StringVar(&proxy, "proxy", "", "HTTP proxy URL (e.g. http://localhost:7890)")
	cmd.Flags().IntVar(&timeout, "timeout", 300, "Login timeout in seconds")

	return cmd
}

func runWeixinOnboard(baseURL, proxy string, timeout time.Duration) error {
	fmt.Println("Starting Weixin (WeChat personal) login...")
	fmt.Println()

	botToken, userID, accountID, returnedBaseURL, err := weixin.PerformLoginInteractive(
		context.Background(),
		weixin.AuthFlowOpts{
			BaseURL: baseURL,
			Timeout: timeout,
			Proxy:   proxy,
		},
	)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	fmt.Println()
	fmt.Println("✅ Login successful!")
	fmt.Printf("   Account ID : %s\n", accountID)
	if userID != "" {
		fmt.Printf("   User ID    : %s\n", userID)
	}
	fmt.Println()

	// Prefer the server-returned base URL (may be region-specific)
	effectiveBaseURL := returnedBaseURL
	if effectiveBaseURL == "" {
		effectiveBaseURL = baseURL
	}

	if err := saveWeixinConfig(botToken, accountID, effectiveBaseURL, proxy); err != nil {
		fmt.Printf("⚠️  Could not auto-save to config: %v\n", err)
		printManualWeixinConfig(botToken, accountID, effectiveBaseURL)
		return nil
	}

	fmt.Println("✓ Config updated. Start the gateway with:")
	fmt.Println()
	fmt.Println("  picoclaw gateway")
	fmt.Println()
	fmt.Println("To restrict which WeChat users can send messages, add their user IDs")
	fmt.Println("to channels.weixin.allow_from in your config.")

	return nil
}

// saveWeixinConfig saves the verified iLink account ID alongside its token.
func saveWeixinConfig(token, accountID, baseURL, proxy string) error {
	cfgPath := internal.GetConfigPath()

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	bc := cfg.Channels.GetByType(config.ChannelWeixin)
	if bc == nil {
		bc = &config.Channel{Type: config.ChannelWeixin}
		cfg.Channels[config.ChannelWeixin] = bc
	}
	bc.Enabled = true

	decoded, err := bc.GetDecoded()
	if err != nil {
		return err
	}
	weixinCfg, ok := decoded.(*config.WeixinSettings)
	if !ok {
		return fmt.Errorf("cannot load Weixin channel settings")
	}
	weixinCfg.SetToken(token)
	weixinCfg.AccountID = accountID
	const defaultBase = "https://ilinkai.weixin.qq.com/"
	if baseURL != "" && baseURL != defaultBase {
		weixinCfg.BaseURL = baseURL
	}
	if proxy != "" {
		weixinCfg.Proxy = proxy
	}

	return config.SaveConfig(cfgPath, cfg)
}

func printManualWeixinConfig(token, accountID, baseURL string) {
	fmt.Println()
	fmt.Println("Add the following to channel_list in your picoclaw config:")
	fmt.Println()
	fmt.Println(`  "weixin": {`)
	fmt.Println(`    "type": "weixin",`)
	fmt.Println(`    "enabled": true,`)
	fmt.Println(`    "settings": {`)
	fmt.Printf("      \"token\": %q,\n", token)
	fmt.Printf("      \"account_id\": %q", accountID)
	const defaultBase = "https://ilinkai.weixin.qq.com/"
	if baseURL != "" && baseURL != defaultBase {
		fmt.Printf(",\n      \"base_url\": %q", baseURL)
	}
	fmt.Println()
	fmt.Println(`    },`)
	fmt.Println(`    "allow_from": []`)
	fmt.Println(`  }`)
}
