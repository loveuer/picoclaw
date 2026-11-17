# Troubleshooting this fork

## Model alias not found

`agents.defaults.model_name` must match an enabled entry's `model_name` in `model_list`. It is a local alias, separate from the remote model ID.

## Unsupported provider or rejected model ID

Only `openai` and `anthropic` chat protocols remain. For compatible endpoints, specify the supported protocol, complete endpoint and remote model ID explicitly:

```json
{
  "provider": "openai",
  "api_base": "https://openrouter.ai/api/v1",
  "model": "vendor/your-model-id",
  "api_keys": ["YOUR_API_KEY"]
}
```

The example fields belong on a `model_list` entry. Replace the remote ID with one supported by your service. With an explicit provider, the complete `model` string is preserved. `provider: "openrouter"` and other vendor presets have been removed. See the [provider guide](../guides/providers.md).

## Removed channel prevents startup

Remove obsolete entries from both `config.json` and `.security.yml`, including old `channels` or current `channel_list` mappings. Disabling an unknown channel type is insufficient. Current channel credentials belong under `channel_list.<name>.settings`.

Obsolete `devices`, `tools.i2c`, `tools.spi`, and `tools.serial` settings are ignored and disappear when the configuration is saved. See the [upgrade notes](../../README.md#从旧版或上游配置升级).

## Configuration edits do not take effect

Check `PICOCLAW_HOME` and `PICOCLAW_CONFIG`, and inspect the paths printed by `picoclaw status`. A `.security.yml` beside the selected config file may override JSON values. Restart the Gateway when changing settings unless hot reload is enabled for the relevant change.

`status` reports configuration and stored authentication state; it does not probe model connectivity or service health. Gateway exposes `/health` and `/ready` on its configured HTTP address.

## Office runtime missing

Run `office.sh setup` and `office.sh check` as the Gateway user. See the [Office runtime guide](../guides/office-skills.zh.md). Updating the binary does not overwrite existing workspace Skills.

## Cron CLI

The current `picoclaw cron add` command supports recurring schedules through `--every` and `--cron`. One-shot reminders are available through the cron tool, rather than a CLI `at_seconds` flag.
