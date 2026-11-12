# PicoClaw · 精简 fork

这是 [sipeed/picoclaw](https://github.com/sipeed/picoclaw) 的个人 fork，使用 Go 构建，通过命令行或 IM 运行 AI 助手。当前版本以 CLI、Gateway 和常用消息渠道为主，已移除网页管理界面与一批渠道及其依赖。

本仓库的功能、配置和构建产物与上游不同，使用时请以本仓库代码和说明为准。

## 当前功能

- **CLI 与 Gateway**：本地交互、单次提问，或通过消息渠道持续运行。
- **模型接入**：支持多种模型提供商、OpenAI 兼容接口、多模型配置与切换。
- **会话管理**：保存对话上下文，支持摘要压缩，以及通过 `/new` 清空上下文后重新提问。
- **工具与扩展**：文件操作、命令执行、网页搜索与抓取、MCP、Skills 和定时任务。
- **语音能力**：保留语音识别与合成，实际可用能力取决于模型、服务及渠道配置。

已移除 WebUI、launcher、桌面托盘及相关前端构建；网页搜索和网页抓取工具仍然保留。

## 消息渠道

目前支持 7 种 IM 渠道和 MQTT：

| 配置类型 | 渠道 | 接入方式与文档 |
| --- | --- | --- |
| `weixin` | 个人微信 | [iLink 接口、扫码登录](docs/channels/weixin/README.zh.md) |
| `wecom` | 企业微信 | [AI Bot WebSocket](docs/channels/wecom/README.zh.md) |
| `feishu` | 飞书 / Lark | [WebSocket 长连接](docs/channels/feishu/README.zh.md) |
| `dingtalk` | 钉钉 | [Stream 模式](docs/channels/dingtalk/README.zh.md) |
| `qq` | QQ 官方机器人 | [官方机器人接口](docs/channels/qq/README.zh.md) |
| `onebot` | OneBot v11 | [WebSocket，需外部 OneBot 实现](docs/channels/onebot/README.zh.md) |
| `telegram` | Telegram | [Bot 长轮询](docs/channels/telegram/README.zh.md) |
| `mqtt` | MQTT | [Broker 发布 / 订阅](docs/channels/mqtt/README.zh.md) |

各渠道支持的消息类型、群聊触发方式和权限配置不同，请按对应文档配置。

## 从源码构建

需要 Git、Make，以及满足 [go.mod](go.mod) 要求的 Go 工具链（当前为 Go 1.25.13）。

```bash
git clone https://github.com/loveuer/picoclaw.git
cd picoclaw
make build
./build/picoclaw version
```

`make build` 会生成内嵌资源，默认使用 `stdjson` 构建标签并关闭 CGO。当前平台的可执行文件入口为 `build/picoclaw`。

交叉编译 Linux amd64：

```bash
make build GOOS=linux GOARCH=amd64
# 产物：build/picoclaw-linux-amd64
```

如需使用 `picoclaw-beta` 名称，同时指定命令源码目录：

```bash
make build GOOS=linux GOARCH=amd64 \
  BINARY_NAME=picoclaw-beta CMD_DIR=cmd/picoclaw
# 产物：build/picoclaw-beta-linux-amd64
```

## 配置与运行

### 1. 初始化

```bash
./build/picoclaw onboard
```

默认使用以下路径，可通过 `PICOCLAW_HOME` 更改数据目录：

| 路径 | 用途 |
| --- | --- |
| `~/.picoclaw/config.json` | 模型、渠道、工具等配置 |
| `~/.picoclaw/.security.yml` | 密钥等敏感配置，加载时与主配置合并 |
| `~/.picoclaw/workspace/` | 工作区与助手上下文文件 |

### 2. 配置模型

编辑 `~/.picoclaw/config.json`。下面是仅使用 CLI 的最小配置示例，请替换模型 ID 和 API Key：

```json
{
  "version": 3,
  "agents": {
    "defaults": {
      "model_name": "main"
    }
  },
  "model_list": [
    {
      "model_name": "main",
      "provider": "openai",
      "model": "your-model-id",
      "api_keys": ["YOUR_API_KEY"]
    }
  ]
}
```

`model_name` 是本地别名，`agents.defaults.model_name` 用它选择默认模型；`provider` 指定提供商，`model` 是远端模型 ID。使用兼容接口时，可在该模型条目中添加 `api_base`。

已有 `.security.yml` 中的值可能覆盖主配置中的同名设置。修改密钥时请同时检查该文件。完整字段示例见 [config/config.example.json](config/config.example.json)，提供商说明见[模型配置文档](docs/guides/providers.zh.md)。

### 3. 使用 CLI

```bash
# 单次提问
./build/picoclaw agent -m "帮我整理今天的待办"

# 交互模式
./build/picoclaw agent
```

### 4. 启动 Gateway

按上面的渠道文档，在配置中启用并填写至少一个渠道。微信和企业微信也可通过 CLI 扫码配置：

```bash
./build/picoclaw auth weixin
# 或
./build/picoclaw auth wecom
```

配置完成后启动：

```bash
./build/picoclaw gateway
```

Gateway 会连接已启用的渠道。可用 `./build/picoclaw status` 查看状态，或用 `./build/picoclaw --help` 查看其他命令。

## 在 IM 中开始新对话

发送以下命令可以丢弃当前会话的历史消息和摘要：

```text
/new
```

它等同于 `/clear`，只清理上下文，不调用模型。也可以在同一条消息里直接带上新问题：

```text
/new 帮我查查今天成都的天气
/new 看看今天有啥新咖啡
```

此时先清理上下文，再执行新问题；后续消息继续这段新对话。这是重置当前会话的模型上下文，不会创建一个可单独切回的历史会话，也不会删除工作区文件或长期记忆。

如果当前会话还有运行中或待处理的任务，`/new` 会提示稍后重试，不会中断任务或重置上下文。

## 从旧版或上游配置升级

已移除的渠道类型如下：

- `pico`、`pico_client`、`maixcam`
- `slack`、`slack_webhook`、`teams_webhook`、`matrix`、`deltachat`、`irc`、`vk`
- `line`、`whatsapp`、`whatsapp_native`、`discord`

升级时，需要从 `config.json` **和** `.security.yml` 的 `channels` 中删除这些渠道条目。仅设置 `enabled: false` 仍可能因未知渠道类型而导致启动失败，例如：

```text
channel "maixcam" has unknown type "maixcam"
```

不再使用上游的 WebUI、launcher 或相关构建命令。部署时请使用本 fork 构建的二进制，避免混用上游发行产物。

## 更多说明

- [配置指南](docs/guides/configuration.zh.md)
- [会话管理](docs/guides/session-guide.zh.md)
- [工具配置](docs/reference/tools_configuration.zh.md)
- [MCP 命令](docs/reference/mcp-cli.md)
- [定时任务](docs/reference/cron.md)

部分专题文档沿用上游内容；如与本页的功能范围不一致，以当前源码和配置示例为准。

开发检查：

```bash
go test -tags stdjson ./...
make lint-docs
```

## 来源与许可

本项目基于 [sipeed/picoclaw](https://github.com/sipeed/picoclaw) 修改，感谢上游及其贡献者。继续使用 [MIT License](LICENSE)，版权声明见许可文件。Go module 路径保留为 `github.com/sipeed/picoclaw`。
