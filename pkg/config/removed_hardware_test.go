package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadConfig_IgnoresRemovedHardwareSettings(t *testing.T) {
	for version := 0; version <= CurrentVersion; version++ {
		t.Run(strconv.Itoa(version), func(t *testing.T) {
			mustSetupSSHKey(t)
			path := filepath.Join(t.TempDir(), "config.json")
			data, err := json.Marshal(map[string]any{
				"version": version,
				"devices": map[string]bool{"enabled": true, "monitor_usb": true},
				"tools": map[string]any{
					"i2c":    map[string]bool{"enabled": true},
					"spi":    map[string]bool{"enabled": true},
					"serial": map[string]bool{"enabled": true},
				},
				"gateway": map[string]int{"port": 18799},
			})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, data, 0o600))

			cfg, err := LoadConfig(path)
			require.NoError(t, err)
			require.True(t, cfg.Tools.Serial.Enabled)
			require.Equal(t, 18799, cfg.Gateway.Port)
			require.NoError(t, SaveConfig(path, cfg))
			data, err = os.ReadFile(path)
			require.NoError(t, err)
			var saved map[string]any
			require.NoError(t, json.Unmarshal(data, &saved))
			require.NotContains(t, saved, "devices")
			savedTools, ok := saved["tools"].(map[string]any)
			require.True(t, ok)
			require.NotContains(t, savedTools, "i2c")
			require.NotContains(t, savedTools, "spi")
			require.Contains(t, savedTools, "serial")
		})
	}
}
