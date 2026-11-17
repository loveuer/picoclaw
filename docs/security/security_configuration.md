# Configuration and credentials

PicoClaw loads `config.json` and a `.security.yml` beside that JSON file. The security file overrides corresponding secret values in the main configuration. `PICOCLAW_HOME` changes the default data directory, including the default config path; `PICOCLAW_CONFIG` selects a different JSON file and therefore the location of its companion security file.

Keep both files private until you have inspected them. Saving configuration separates supported secret fields into `.security.yml`, but manually written JSON, custom headers, and other arbitrary values may still contain credentials.

## Main configuration

Use the current schema shown in [config/config.example.json](../../config/config.example.json):

```json
{
  "version": 3,
  "agents": {
    "defaults": {"model_name": "main"}
  },
  "model_list": [
    {
      "model_name": "main",
      "provider": "openai",
      "model": "your-model-id",
      "api_base": "https://api.openai.com/v1",
      "enabled": true
    }
  ],
  "channel_list": {
    "telegram": {
      "enabled": true,
      "type": "telegram",
      "allow_from": ["YOUR_USER_ID"],
      "settings": {}
    }
  }
}
```

`enabled`, `type`, `allow_from`, and other common channel fields belong on the channel entry. Token, App ID, proxy and other channel-specific fields belong under `settings`. App IDs and other non-secret settings stay in `config.json`; supported secret fields can be stored in the companion file.

## Companion security file

```yaml
model_list:
  main:
    api_keys:
      - "YOUR_MODEL_API_KEY"
channel_list:
  telegram:
    settings:
      token: "YOUR_TELEGRAM_BOT_TOKEN"
  feishu:
    settings:
      app_secret: "YOUR_FEISHU_APP_SECRET"
  wecom:
    settings:
      secret: "YOUR_WECOM_SECRET"
  mqtt:
    settings:
      username: "YOUR_MQTT_USERNAME"
      password: "YOUR_MQTT_PASSWORD"
web:
  brave:
    api_keys:
      - "YOUR_BRAVE_API_KEY"
```

Keep only entries for models, channels, and tools you actually configure. Model credentials use a **mapping** keyed by the local `model_name`, not the JSON model list. Saved files can use indexed names such as `main:0`; loading also supports the base name `main`. Model keys use `api_keys` arrays, even for one key. This is direct field mapping, without `ref:` references.

All current channel credentials use the nested `channel_list.<name>.settings` structure. Legacy `channels` is accepted during migration; it should not be used in new examples. The channel name must match the corresponding main-config entry. Settings for unknown or removed channels must be removed from both files; see the [upgrade notes](../../README.md#从旧版或上游配置升级).

Chat protocols are `openai` and `anthropic`. Compatible services require an explicit supported protocol, endpoint, and model ID. Search credentials under `web` are separate from chat model providers. See the [provider guide](../guides/providers.md) and [tool configuration](../reference/tools_configuration.md).

## Authentication and encryption

OpenAI OAuth and Anthropic setup-token credentials use `auth.json` under `PICOCLAW_HOME`. Manage them with `picoclaw auth login`, `auth status`, and `auth logout`; keep that file private too.

For optional encrypted credential storage, follow [Credential Encryption](credential_encryption.md). Do not copy encryption settings or encrypted values between installations without the required key material.

## Checking changes

`picoclaw status` shows the selected config path and model credential state, but does not test API connectivity or Gateway health. Check the service logs after restarting and use `/health` and `/ready` on the Gateway HTTP address.

If a credential change in JSON appears ineffective, inspect the companion `.security.yml` for an overriding value. Avoid pasting either file into issue reports; use redacted examples.
