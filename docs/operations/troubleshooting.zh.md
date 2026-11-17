# 疑难解答

## 找不到模型或提供商不支持

`agents.defaults.model_name` 必须对应 `model_list` 中已启用条目的 `model_name`。它是本地别名，不是远端模型 ID。

聊天模型仅支持 `openai` 和 `anthropic`。第三方兼容服务应显式填写协议、`api_base` 和完整远端 `model`，不要继续使用 `openrouter`、`gemini`、`ollama` 等旧 provider 名称。显式指定协议后，带厂商命名空间的模型 ID 会原样发送；参见[模型配置](../guides/providers.zh.md)。

## 升级后渠道启动失败

从 `config.json` 和 `.security.yml` 中删除已移除的渠道条目，检查当前 `channel_list` 和旧版 `channels` 两种名称。仅设置 `enabled: false` 无法解决未知渠道类型。渠道密钥等专有字段应放在 `channel_list.<名称>.settings` 中。

旧的 `devices`、`tools.i2c`、`tools.spi`、`tools.serial` 会兼容忽略，保存后不再输出。详细清单见 [README 升级说明](../../README.md#从旧版或上游配置升级)。

## 配置修改没有生效

用 `picoclaw status` 检查实际配置路径，注意 `PICOCLAW_HOME`、`PICOCLAW_CONFIG` 及配置文件旁的 `.security.yml`。同名密钥可能被安全文件覆盖。没有启用对应热重载能力时，修改配置后重启 Gateway。

`status` 展示配置和凭据状态，不验证模型连通性或服务健康。运行状态请检查服务日志，以及 Gateway HTTP 地址上的 `/health`、`/ready`。

## 办公文件环境不可用

以 Gateway 用户执行 `office.sh setup` 和 `office.sh check`，见[办公环境说明](../guides/office-skills.zh.md)。更新二进制不会自动覆盖已有工作区 Skill。

## 定时任务 CLI

`picoclaw cron add` 支持 `--every`、`--cron` 周期任务。一次性提醒通过 cron 工具设置，当前没有 CLI `at_seconds` 参数。
