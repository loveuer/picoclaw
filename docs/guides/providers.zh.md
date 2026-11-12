# 模型接入

本 fork 仅保留 OpenAI 和 Anthropic 两种模型协议。OpenAI 支持 Responses API、Chat Completions 和流式输出；Anthropic 使用原生 Messages API，支持工具调用和思考参数。

## API Key 配置

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
      "api_keys": ["YOUR_ANTHROPIC_API_KEY"],
      "thinking_level": "high"
    }
  ]
}
```

`model_name` 是本地别名，`model` 是远端模型 ID。显式填写 `provider` 时，`model` 会原样交给对应协议处理；推荐直接填写远端要求的 ID。未填写 `provider` 时，仍识别 `openai/`、`anthropic/` 前缀，普通模型名默认使用 OpenAI。

`gpt`、`claude` 分别是两种协议的别名，旧的 `anthropic-messages` 也归一化为 `anthropic`。

## Responses 与兼容端点

使用 OpenAI 官方端点时优先调用 `/responses`；自定义端点上，GPT 模型（不含 `gpt-oss`）也优先使用 Responses。端点明确不支持 Responses 时回退到 `/chat/completions`；其他兼容模型使用 Chat Completions。授权、限流等错误不会任意切换接口，具体回退条件由实现判定。

第三方服务只要提供兼容 API，仍可使用：选择 `provider: "openai"` 或 `"anthropic"`，设置 `api_base` 和完整远端 `model`。例如模型 ID 可以包含 `vendor/model`，显式填写协议可避免它被当成协议前缀。

兼容端点不再提供厂商默认 URL 或厂商专用 provider 名称。原来使用 `deepseek`、`openrouter`、`ollama` 等名称的配置，需要改为 `openai` 并显式填写端点；原生 Gemini、Azure、Bedrock、Antigravity、Copilot 和 CLI 代理已移除。不能将非兼容 API 仅靠改名称变成 OpenAI / Anthropic API。

保留 `proxy`、`request_timeout`（秒）、`custom_headers`、`extra_body`、`user_agent`、`thinking_level`、`tool_schema_transform`，以及多模型、多个 API Key、限速和 fallback。`extra_body` / `custom_headers` 可覆盖请求参数，请按实际服务配置。两种协议均支持 `streaming`；Anthropic 登录令牌也沿用其内部流式请求处理。语音专用 ASR/TTS 服务独立选择，不属于聊天模型提供商。

## 登录与令牌

```bash
picoclaw auth login --provider openai
picoclaw auth login --provider openai --device-code
picoclaw auth login --provider anthropic --setup-token
picoclaw auth status
picoclaw auth logout --provider openai
```

OpenAI OAuth 和 Anthropic setup-token 登录仍保留，通过 `auth_method: "oauth"` 或 `"token"` 选择现有登录凭据。它们与 API Key 接入使用不同的认证路径，第三方兼容端点通常应使用 API Key。

微信、企业微信的扫码登录不受模型提供商精简影响。原来的 Antigravity 登录和 `auth models` 命令已移除；模型查询可使用 `model add` 的兼容接口流程。

密钥等敏感设置可能来自 `.security.yml`，修改配置时请同时检查。完整字段见 [配置示例](../../config/config.example.json)，部署说明见 [README](../../README.md)。
