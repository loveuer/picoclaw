# PicoClaw · 精简 fork

这是 [sipeed/picoclaw](https://github.com/sipeed/picoclaw) 的个人 fork，使用 Go 构建，通过命令行或 IM 运行 AI 助手。当前版本以 CLI、Gateway 和常用消息渠道为主，已移除网页管理界面与一批渠道及其依赖。

本仓库的功能、配置和构建产物与上游不同，使用时请以本仓库代码和说明为准。

## 当前功能

- **CLI 与 Gateway**：本地交互、单次提问，或通过消息渠道持续运行。
- **模型接入**：仅保留 OpenAI（Responses / Chat Completions）和 Anthropic Messages，支持兼容端点、多模型配置与切换。
- **会话管理**：保存对话上下文，支持摘要压缩，以及通过 `/new` 清空上下文后重新提问。
- **工具与扩展**：文件操作、命令执行、网页搜索与抓取、MCP、Skills 和定时任务。
- **语音能力**：保留语音识别与合成，实际可用能力取决于模型、服务及渠道配置。

已移除 WebUI、launcher、桌面托盘及相关前端构建；网页搜索和网页抓取工具仍然保留。

已移除 I²C、SPI、USB 设备监控及内置 hardware Skill；`serial` 串口工具保留，默认关闭。

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

## 检查与发布

GitHub 的 `CI` 工作流在推送 `main`、提交 PR 或手动触发时运行，保留测试、文档检查、vet、lint、漏洞检查、Docker 集成测试和 Linux amd64 构建。

`Release` 仅手动触发，发布到当前仓库的 GitHub Releases，使用内置 `GITHUB_TOKEN`。自动发布目标收敛为 Linux amd64，不再运行定时 nightly、多平台发布、Docker Hub / TOS 上传或 deb/rpm 打包；本地 Docker 环境和交叉编译命令仍可使用。

发布稳定版时，先提交包含发布脚本的代码，创建并推送版本标签，例如：

```bash
git tag v1.0.0
git push origin v1.0.0
```

然后在 GitHub Actions 中手动运行 `Release`，选择 `channel=stable`、`tag=v1.0.0`。工作流会检出该标签，测试并构建后创建稳定版 Release，设为 latest。标签必须指向包含新发布脚本的提交；已发布版本不覆盖。

发布测试版时，选择需要构建的分支或标签，设置 `channel=nightly`，将 `tag` 留空。测试和打包成功后会替换 `nightly` Release 及标签，标记为预发布，不影响 latest 稳定版。

两种发布均上传 `picoclaw_Linux_x86_64.tar.gz` 和 `checksums.txt`。压缩包内含 `picoclaw` 与 `LICENSE`，可供下面的自更新命令使用。也可在本地生成同样的发布包：

```bash
bash scripts/package-release.sh v1.0.0
# 产物位于 dist/，可通过第二个参数指定输出目录
```

## 自更新

`update` 默认从本 fork 的 `loveuer/picoclaw` GitHub Releases 获取更新，不会自动回退到上游。

```bash
# 检查稳定版发布产物和校验信息，不下载二进制、不替换文件
picoclaw update --check

# 应用稳定版；也可将命令名换成实际安装的 picoclaw-beta
picoclaw update

# 检查 / 应用本 fork 的 nightly Release
picoclaw update --nightly --check
picoclaw update --nightly

# 显式选择其他 fork 或固定版本
picoclaw update --repo owner/repo --check
picoclaw update --url https://github.com/loveuer/picoclaw/releases/tag/v1.0.0 --check
```

`--url` 支持 GitHub 仓库、Release 页面或 Releases API 地址，与 `--repo`、`--nightly` 互斥。不会把无效地址当成默认源。`--check` 检查产物可用性，不比较版本新旧；可配合 `--platform`、`--arch` 查询其他平台，实际自更新仅允许当前系统与架构。

发布更新时需要先在本 fork 创建稳定版 Release 或名为 `nightly` 的 Release，并上传对应平台的 `.tar.gz`、`.tgz`、`.tar` 或 `.zip` 包。例如 `picoclaw_Linux_x86_64.tar.gz`，内部包含 `picoclaw` 或 `picoclaw-beta`（Windows 对应 `.exe`）。需要 GitHub 提供的 SHA256 digest，或包含产物文件名与 SHA256 的 checksums 文件。单独的二进制下载链接目前不支持。

更新会校验下载文件并替换当前可执行文件，保留实际安装名称，需要对安装目录有写权限。沿用现有回滚机制保留旧 binary 为 `.old`；不会修改用户配置、自动重启服务或覆盖工作区 Skill。更新后按部署方式重启；需要刷新内置 Skill 时，再使用 `skills install-builtin`。

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

`model_name` 是本地别名，`agents.defaults.model_name` 用它选择默认模型；`provider` 指定提供商，`model` 是远端模型 ID。使用兼容接口时，仍选择 `provider: "openai"` 或 `"anthropic"`，并显式填写 `api_base` 和远端 `model`。其他厂商专用名称、原生 Gemini / Azure / Bedrock、Copilot、Antigravity 和 CLI 代理已移除；原有配置需要按协议调整。

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

旧配置中的 `devices`、`tools.i2c` 和 `tools.spi` 字段会被忽略，保存配置时不再输出。已有工作区的 `skills/hardware` 不会自动删除，如曾安装过，请手动移除该目录。

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
