// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package config

import (
	"encoding/json"
	"path/filepath"

	"github.com/sipeed/picoclaw/pkg"
)

// DefaultConfig returns the default configuration for PicoClaw.
func DefaultConfig() *Config {
	workspacePath := filepath.Join(GetHome(), pkg.WorkspaceName)

	return &Config{
		Version: CurrentVersion,
		// Isolation is opt-in so existing installations keep their current behavior
		// until the user explicitly enables subprocess sandboxing.
		Isolation: IsolationConfig{
			Enabled: false,
		},
		Agents: AgentsConfig{
			Defaults: AgentDefaults{
				Workspace:                 workspacePath,
				RestrictToWorkspace:       true,
				Provider:                  "",
				MaxTokens:                 32768,
				Temperature:               nil, // nil means use provider default
				MaxToolIterations:         50,
				SummarizeMessageThreshold: 20,
				SummarizeTokenPercent:     75,
				SteeringMode:              "one-at-a-time",
				ToolFeedback: ToolFeedbackConfig{
					Enabled:          false,
					MaxArgsLength:    300,
					SeparateMessages: false,
				},
				SplitOnMarker:       false,
				MaxLLMRetries:       2,
				LLMRetryBackoffSecs: 2,
			},
		},
		Session: SessionConfig{
			Dimensions: []string{"chat"},
		},
		Evolution: EvolutionConfig{
			Enabled:         false,
			Mode:            "observe",
			MinTaskCount:    2,
			MinSuccessRatio: 0.7,
			ColdPathTrigger: "after_turn",
		},
		Channels: defaultChannels(),
		Hooks: HooksConfig{
			Enabled: true,
			Defaults: HookDefaultsConfig{
				ObserverTimeoutMS:    500,
				InterceptorTimeoutMS: 5000,
				ApprovalTimeoutMS:    60000,
			},
		},
		ModelList: []*ModelConfig{
			{ModelName: "gpt-5.4", Provider: "openai", Model: "gpt-5.4", APIBase: "https://api.openai.com/v1"},
			{
				ModelName: "claude-sonnet-4.6",
				Provider:  "anthropic",
				Model:     "claude-sonnet-4-6",
				APIBase:   "https://api.anthropic.com/v1",
			},
		},
		Gateway: GatewayConfig{
			Host:      "localhost",
			Port:      18790,
			HotReload: false,
			LogLevel:  DefaultGatewayLogLevel,
		},
		Events: EventsConfig{
			Logging: defaultEventLoggingConfig(),
		},
		Tools: ToolsConfig{
			FilterSensitiveData: true,
			FilterMinLength:     8,
			MediaCleanup: MediaCleanupConfig{
				ToolConfig: ToolConfig{
					Enabled: true,
				},
				MaxAge:   30,
				Interval: 5,
			},
			Web: WebToolsConfig{
				ToolConfig: ToolConfig{
					Enabled: true,
				},
				Provider:        "auto",
				PreferNative:    true,
				Proxy:           "",
				FetchLimitBytes: 10 * 1024 * 1024, // 10MB by default
				Format:          "plaintext",
				Brave: BraveConfig{
					Enabled:    false,
					MaxResults: 5,
				},
				Tavily: TavilyConfig{
					Enabled:    false,
					MaxResults: 5,
				},
				Kagi: KagiConfig{
					Enabled:    false,
					BaseURL:    "https://kagi.com/api/v1/search",
					MaxResults: 5,
				},
				Sogou: SogouConfig{
					Enabled:    true,
					MaxResults: 5,
				},
				DuckDuckGo: DuckDuckGoConfig{
					Enabled:    false,
					MaxResults: 5,
				},
				Gemini: GeminiSearchConfig{
					Enabled:    false,
					Model:      "gemini-2.5-flash",
					MaxResults: 5,
				},
				Perplexity: PerplexityConfig{
					Enabled:    false,
					MaxResults: 5,
				},
				SearXNG: SearXNGConfig{
					Enabled:    false,
					BaseURL:    "",
					MaxResults: 5,
				},
				GLMSearch: GLMSearchConfig{
					Enabled:      false,
					BaseURL:      "https://open.bigmodel.cn/api/paas/v4/web_search",
					SearchEngine: "search_std",
					MaxResults:   5,
				},
				BaiduSearch: BaiduSearchConfig{
					Enabled:    false,
					BaseURL:    "https://qianfan.baidubce.com/v2/ai_search/web_search",
					MaxResults: 10,
				},
			},
			Cron: CronToolsConfig{
				ToolConfig: ToolConfig{
					Enabled: true,
				},
				ExecTimeoutMinutes: 5,
				AllowCommand:       true,
			},
			Exec: ExecConfig{
				ToolConfig: ToolConfig{
					Enabled: true,
				},
				EnableDenyPatterns: true,
				AllowRemote:        true,
				TimeoutSeconds:     60,
			},
			Skills: SkillsToolsConfig{
				ToolConfig: ToolConfig{
					Enabled: true,
				},
				Registries: SkillsRegistriesConfig{
					&SkillRegistryConfig{
						Name:    "clawhub",
						Enabled: true,
						BaseURL: "https://clawhub.ai",
						Param:   map[string]any{},
					},
					&SkillRegistryConfig{
						Name:    "github",
						Enabled: true,
						BaseURL: "https://github.com",
						Param:   map[string]any{},
					},
				},
				MaxConcurrentSearches: 2,
				SearchCache: SearchCacheConfig{
					MaxSize:    50,
					TTLSeconds: 300,
				},
			},
			SendFile: ToolConfig{
				Enabled: true,
			},
			SendTTS: ToolConfig{
				Enabled: false,
			},
			MCP: MCPConfig{
				ToolConfig: ToolConfig{
					Enabled: false,
				},
				Discovery: ToolDiscoveryConfig{
					Enabled:          false,
					TTL:              5,
					MaxSearchResults: 5,
					UseBM25:          true,
					UseRegex:         false,
				},
				MaxInlineTextChars: DefaultMCPMaxInlineTextChars,
				Servers:            map[string]MCPServerConfig{},
			},
			AppendFile: ToolConfig{
				Enabled: true,
			},
			EditFile: ToolConfig{
				Enabled: true,
			},
			FindSkills: ToolConfig{
				Enabled: true,
			},
			InstallSkill: ToolConfig{
				Enabled: true,
			},
			ListDir: ToolConfig{
				Enabled: true,
			},
			LoadImage: ToolConfig{
				Enabled: true,
			},
			Message: MessageToolsConfig{
				ToolConfig: ToolConfig{
					Enabled: true,
				},
				MediaEnabled: false,
			},
			ReadFile: ReadFileToolConfig{
				Enabled:         true,
				Mode:            ReadFileModeBytes,
				MaxReadFileSize: 64 * 1024, // 64KB
			},
			Serial: ToolConfig{
				Enabled: false, // Hardware tool - requires host serial ports
			},
			Spawn: ToolConfig{
				Enabled: true,
			},
			SpawnStatus: ToolConfig{
				Enabled: false,
			},
			Subagent: ToolConfig{
				Enabled: true,
			},
			WebFetch: ToolConfig{
				Enabled: true,
			},
			WriteFile: ToolConfig{
				Enabled: true,
			},
		},
		Heartbeat: HeartbeatConfig{
			Enabled:  true,
			Interval: 30,
		},
		Voice: VoiceConfig{
			ModelName:         "",
			TTSModelName:      "",
			EchoTranscription: false,
			ElevenLabsAPIKey:  "",
		},
		BuildInfo: BuildInfo{
			Version:   Version,
			GitCommit: GitCommit,
			BuildTime: BuildTime,
			GoVersion: GoVersion,
		},
	}
}

func defaultChannels() ChannelsConfig {
	defs := map[string]any{
		"telegram": map[string]any{
			"typing":      map[string]any{"enabled": true},
			"placeholder": map[string]any{"enabled": true, "text": []string{"Thinking... 💭"}},
			"settings": map[string]any{
				"use_markdown_v2":      false,
				"media_group_delay_ms": 500,
			},
		},
		"feishu": map[string]any{},
		"qq": map[string]any{
			"settings": map[string]any{"max_message_length": 2000},
		},
		"dingtalk": map[string]any{},
		"onebot": map[string]any{
			"settings": map[string]any{
				"ws_url":             "ws://127.0.0.1:3001",
				"reconnect_interval": 5,
			},
		},
		"wecom": map[string]any{
			"settings": map[string]any{
				"websocket_url":         "wss://openws.work.weixin.qq.com",
				"send_thinking_message": true,
			},
		},
		"weixin": map[string]any{
			"settings": map[string]any{
				"base_url":     "https://ilinkai.weixin.qq.com/",
				"cdn_base_url": "https://novac2c.cdn.weixin.qq.com/c2c",
			},
		},
	}

	channels := make(ChannelsConfig, len(defs))
	for name, def := range defs {
		data, err := json.Marshal(def)
		if err != nil {
			continue
		}
		bc := &Channel{}
		if err := json.Unmarshal(data, bc); err != nil {
			continue
		}
		bc.SetName(name)
		if bc.Type == "" {
			bc.Type = name
		}
		channels[name] = bc
	}
	return channels
}
