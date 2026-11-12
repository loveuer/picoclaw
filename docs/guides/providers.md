# Model providers

This fork supports two model protocols: **OpenAI** and **Anthropic**. OpenAI supports Responses, Chat Completions, and streaming. Anthropic uses its native Messages API with tool calls and thinking controls.

```json
{
  "version": 3,
  "agents": {"defaults": {"model_name": "main"}},
  "model_list": [
    {
      "model_name": "main",
      "provider": "openai",
      "model": "gpt-5.4",
      "api_base": "https://api.openai.com/v1",
      "api_keys": ["YOUR_OPENAI_API_KEY"]
    },
    {
      "model_name": "claude",
      "provider": "anthropic",
      "model": "claude-sonnet-4-6",
      "api_base": "https://api.anthropic.com/v1",
      "api_keys": ["YOUR_ANTHROPIC_API_KEY"]
    }
  ]
}
```

With an explicit `provider`, use the remote model ID, including any vendor namespace. Without it, `openai/` and `anthropic/` prefixes remain supported; bare IDs default to OpenAI. `gpt`, `claude`, and the legacy `anthropic-messages` spelling remain aliases.

OpenAI's official endpoint uses Responses first. Custom endpoints use Responses for GPT models except `gpt-oss`, with fallback to Chat Completions when Responses is unsupported. Other compatible models use Chat Completions. Authentication and rate-limit failures do not trigger arbitrary endpoint fallback.

For third-party compatible services, choose `openai` or `anthropic` and specify `api_base` and the remote `model`. Named vendor presets, native Gemini, Azure, Bedrock, Antigravity, Copilot, and CLI proxies have been removed. Existing named-provider configurations require an explicit supported protocol and endpoint; incompatible APIs cannot be migrated by renaming the provider alone.

HTTP proxy, request timeout, custom headers, extra body fields, user agent, thinking, schema transforms, multiple models/keys, rate limits, and fallback remain available. Both protocols support the `streaming` configuration. Anthropic login tokens also retain their internal streaming request handling. Speech-only ASR/TTS services keep separate selection logic and are not chat providers.

OpenAI OAuth and Anthropic setup-token login remain available:

```bash
picoclaw auth login --provider openai
picoclaw auth login --provider openai --device-code
picoclaw auth login --provider anthropic --setup-token
picoclaw auth status
```

Use `auth_method: "oauth"` or `"token"` to select stored login credentials. API keys are the usual choice for custom endpoints. Weixin and WeCom channel login remain available. Antigravity login and its `auth models` command have been removed.

See the [configuration example](../../config/config.example.json), [Chinese guide](providers.zh.md), and [README](../../README.md). Settings from `.security.yml` may override corresponding values in the main configuration.
