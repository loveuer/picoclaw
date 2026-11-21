package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestSaveWeixinConfigPersistsVerifiedAccountID(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config.json")
	t.Setenv(config.EnvHome, home)
	t.Setenv(config.EnvConfig, path)
	data := `{"version":3,"channel_list":{"weixin":{"type":"weixin","settings":{"account_id":"previous-account"}}}}`
	require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
	require.NoError(t, saveWeixinConfig("test-bot-token", "verified-account", "", ""))

	cfg, err := config.LoadConfig(path)
	require.NoError(t, err)
	channel := cfg.Channels.Get(config.ChannelWeixin)
	require.NotNil(t, channel)
	require.True(t, channel.Enabled)
	settings, err := channel.GetDecoded()
	require.NoError(t, err)
	w, ok := settings.(*config.WeixinSettings)
	require.True(t, ok)
	require.Equal(t, "verified-account", w.AccountID)
}
