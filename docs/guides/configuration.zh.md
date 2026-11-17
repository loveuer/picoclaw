# ⚙️ 配置指南

> 返回 [README](../../README.md)

## ⚙️ 配置详解

配置文件路径: `~/.picoclaw/config.json`

### 环境变量

你可以使用环境变量覆盖默认路径。这对于便携安装、容器化部署或将 picoclaw 作为系统服务运行非常有用。`PICOCLAW_HOME` 也会改变默认配置文件路径；显式指定的 `PICOCLAW_CONFIG` 优先。

| 变量              | 描述                                                                                                                             | 默认路径                  |
|-------------------|-----------------------------------------------------------------------------------------------------------------------------------------|---------------------------|
| `PICOCLAW_CONFIG` | 覆盖配置文件的路径。这直接告诉 picoclaw 加载哪个 `config.json`，忽略所有其他位置。 | `~/.picoclaw/config.json` |
| `PICOCLAW_HOME`   | 覆盖 picoclaw 数据根目录。这会更改 `workspace` 和其他数据目录的默认位置。          | `~/.picoclaw`             |

**示例：**

```bash
# 使用特定的配置文件运行 picoclaw
# 工作区路径将从该配置文件中读取
PICOCLAW_CONFIG=/etc/picoclaw/production.json picoclaw gateway

# 在 /opt/picoclaw 中存储所有数据运行 picoclaw
# 配置将从 /opt/picoclaw/config.json 加载
# 工作区将在 /opt/picoclaw/workspace 创建
PICOCLAW_HOME=/opt/picoclaw picoclaw agent

# 同时使用两者进行完全自定义设置
PICOCLAW_HOME=/srv/picoclaw PICOCLAW_CONFIG=/srv/picoclaw/main.json picoclaw gateway
```

### Gateway 日志等级

`gateway.log_level` 控制 Gateway 的日志详细程度，可在 `config.json` 中配置：

```json
{
  "gateway": {
    "log_level": "warn"
  }
}
```

默认值为 `warn`。支持的值：`debug`、`info`、`warn`、`error`、`fatal`。

也可通过环境变量覆盖：`PICOCLAW_LOG_LEVEL=info`

### 工作区布局 (Workspace Layout)

PicoClaw 将数据存储在您配置的工作区中（默认：`~/.picoclaw/workspace`）：

```
~/.picoclaw/workspace/
├── sessions/          # 对话会话和历史
├── memory/           # 长期记忆 (MEMORY.md)
├── state/            # 持久化状态 (最后一次频道等)
├── cron/             # 定时任务数据库
├── skills/           # 自定义技能
├── AGENT.md          # Agent 行为指南
├── HEARTBEAT.md      # 周期性任务提示词 (每 30 分钟检查一次)
├── IDENTITY.md       # Agent 身份设定
├── SOUL.md           # Agent 灵魂/性格
└── USER.md           # 用户偏好
```

> **提示：** 对 `AGENT.md`、`SOUL.md`、`USER.md` 和 `memory/MEMORY.md` 的修改会通过文件修改时间（mtime）在运行时自动检测。**无需重启 gateway**，Agent 将在下一次请求时自动加载最新内容。

### Agent 自进化

`evolution` 配置块控制 PicoClaw 的自进化运行时。启用后，Agent 会把已完成的回合记录为学习记录。在更高模式下，它可以聚类重复出现的成功模式、生成技能草稿，并可选择把已接受的草稿应用到工作区技能中。

```json
{
  "evolution": {
    "enabled": false,
    "mode": "observe",
    "state_dir": "",
    "min_task_count": 2,
    "min_success_ratio": 0.7,
    "cold_path_trigger": "after_turn",
    "cold_path_times": []
  }
}
```

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `enabled` | `false` | 启用已完成 Agent 回合的学习记录采集。Heartbeat 回合会被忽略。 |
| `mode` | `observe` | `observe` 只记录数据；`draft` 可生成候选技能草稿；`apply` 可将已接受草稿应用到工作区技能。 |
| `state_dir` | `""` | 自进化状态的可选目录。留空时使用工作区下的默认位置。 |
| `min_task_count` | `2` | 一个模式具备生成草稿资格前所需的最小相关任务记录数。 |
| `min_success_ratio` | `0.7` | 任务聚类所需的最小成功率，取值需大于 `0`，且不超过 `1`。 |
| `cold_path_trigger` | `after_turn` | 草稿生成可在 `after_turn` 后运行、按 `scheduled` 定时运行；设置为 `manual` 时会关闭自动冷路径运行。目前还没有用户可用的手动触发入口。仅在 `draft` 和 `apply` 模式下生效。 |
| `cold_path_times` | `[]` | 当 `cold_path_trigger` 为 `scheduled` 时使用的运行时间，格式为 `HH:MM` 字符串。 |

如果你只想先检查学习记录，建议从 `observe` 开始。需要生成可审查改进时使用 `draft`。只有在你接受让已通过的草稿更新工作区技能时，才使用 `apply`。

### 请求上下文策略

`turn_profile` 是 `agents.defaults.turn_profile` 下的可选请求上下文策略，用来控制每个新回合是否带入历史、系统提示、技能提示，以及允许调用哪些工具。不写该配置或设置 `"enabled": false` 时，PicoClaw 完全保持原逻辑；设置 `"enabled": true` 后，下面的策略会应用到每个新回合。

所有块都使用同一组 `mode`：

| Mode | 含义 |
| --- | --- |
| `default` | 保持 PicoClaw 原逻辑。块缺失或 `mode` 缺失都按 `default` 处理。 |
| `off` | 关闭该块。 |
| `custom` | 使用允许列表。本版本仅支持 `skills` 和 `tools`，在 `history` 或 `system_prompt` 中使用会触发配置校验错误。 |

各块含义：

| 块 | 控制内容 |
| --- | --- |
| `history` | 是否读取历史和摘要、写入用户/助手/工具消息、写入 context manager，以及是否压缩或总结本轮会话。 |
| `system_prompt` | 是否注入 PicoClaw 默认身份、工作区指令、记忆、运行时上下文和摘要。关闭后仍可使用外部传入的 system prompt。 |
| `skills` | 是否加载技能目录和 active skill 提示。`custom.allow` 只保留列出的技能名。 |
| `tools` | 暴露给模型并允许执行的工具。`custom.allow` 只保留已注册且列出的工具名。 |

当 `system_prompt.mode` 为 `off`、工具仍可见且没有外部 system prompt 时，PicoClaw 会复用现有的工具使用规则作为最小兜底提示。如果 `tools.mode` 为 `off`，则不会添加兜底提示。

只保留 Web 工具的干净上下文示例：

```json
{
  "agents": {
    "defaults": {
      "turn_profile": {
        "enabled": true,
        "history": { "mode": "off" },
        "system_prompt": { "mode": "off" },
        "skills": { "mode": "off" },
        "tools": {
          "mode": "custom",
          "allow": ["web_search", "web_fetch"]
        }
      }
    }
  }
}
```

### 技能来源 (Skill Sources)

默认情况下，技能会按以下顺序加载：

1. `~/.picoclaw/workspace/skills`（工作区）
2. `~/.picoclaw/skills`（全局）
3. `<构建时嵌入路径>/skills`（内置）

在高级/测试场景下，可通过以下环境变量覆盖内置技能目录：

```bash
export PICOCLAW_BUILTIN_SKILLS=/path/to/skills
```

### 在聊天频道中使用技能

技能安装完成后，可以直接在聊天频道里查看并显式启用它们：

- `/list skills`：显示当前 Agent 可用的已安装技能名称。
- `/use <skill> <message>`：只对当前这一条请求强制使用指定技能。
- `/use <skill>`：为同一会话中的下一条消息预先启用该技能。
- `/use clear`：取消通过 `/use <skill>` 设置的待应用技能。
- `/btw <question>`：发起一个即时的旁支提问，且不改动当前会话历史。`/btw` 会按一次无工具的直接问答处理，不会进入常规的工具执行流程。

示例：

```text
/list skills
/use git explain how to squash the last 3 commits
/btw 帮我回顾一下刚才关于发布方案的结论
/use italiapersonalfinance
dammi le ultime news
```

### 统一命令执行策略

- 通用斜杠命令通过 `pkg/agent/loop.go` 中的 `commands.Executor` 统一执行。
- Channel 适配器不再在本地消费通用命令；它们只负责把入站文本转发到 bus/agent 路径。Telegram 仍会在启动时自动注册其支持的命令菜单，例如 `/start`、`/help`、`/show`、`/list`、`/use` 和 `/btw`。
- 未注册的斜杠命令（例如 `/foo`）会透传给 LLM 按普通输入处理。
- `/new` 与 `/clear` 一样，清除当前会话的历史和摘要。`/new <prompt>` 会先清除，再立即执行后面的新问题，后续消息继续这段新对话。当前模型、工作区和系统提示保持生效；命令与问题之间可以使用空格、制表符或换行分隔。
- 如果同一会话已有正在执行或等待执行的任务，`/new`（包括带新问题的形式）会回复当前任务名称，提示稍后重试；不会打断旧任务，也不会将新问题加入队列。其他会话仍可使用 `/new`。
- 已注册但当前 channel 不支持的命令会返回明确的用户可见错误，并停止后续处理。

### Session 隔离

Session scope 决定了聊天、用户、线程和 space 之间共享多少上下文。

- 全局默认值使用 `session.dimensions`
- 如果只想让某条路由例外，使用 dispatch rule 上的 `session_dimensions`

如果你想看完整的隔离方案和配置配方，请看 [Session 使用指南](session-guide.zh.md)。

### Routing

Routing 通过 `agents.dispatch.rules` 配置。

每条规则都针对 channel 归一化后的 inbound context 做匹配。
规则按从上到下顺序检查，第一条命中的规则立即生效。若没有规则命中，PicoClaw 会回退到默认 agent。

支持的匹配字段：

* `channel`
* `account`
* `space`
* `chat`
* `topic`
* `sender`
* `mentioned`

这些值使用和 session system 一致的归一化词汇：

* `space`: `workspace:t001`、`guild:123456`
* `chat`: `direct:user123`、`group:-100123`、`channel:c123`
* `topic`: `topic:42`
* `sender`: 平台归一化后的 sender 标识

规则也可以通过 `session_dimensions` 覆盖全局 `session.dimensions`，这样路由和会话隔离就能保持一致，而不必回到旧的 `bindings` 或 `dm_scope` 配置。

示例：

```json
{
  "agents": {
    "list": [
      { "id": "main", "default": true },
      { "id": "support" },
      { "id": "sales" }
    ],
    "dispatch": {
      "rules": [
        {
          "name": "vip in support group",
          "agent": "sales",
          "when": {
            "channel": "telegram",
            "chat": "group:-1001234567890",
            "sender": "12345"
          },
          "session_dimensions": ["chat", "sender"]
        },
        {
          "name": "telegram support group",
          "agent": "support",
          "when": {
            "channel": "telegram",
            "chat": "group:-1001234567890"
          },
          "session_dimensions": ["chat"]
        }
      ]
    }
  },
  "session": {
    "dimensions": ["chat"]
  }
}
```

在这个例子里，VIP 规则必须放在更宽泛的群规则前面。
因为 routing 是严格按顺序执行的，所以更具体的规则要放前面，兜底规则放后面。

如果你想看更完整的 agent 路由和模型分层示例，请看 [路由使用指南](routing-guide.zh.md)。

### 🔒 安全沙箱 (Security Sandbox)

PicoClaw 默认在沙箱环境中运行。Agent 只能访问配置的工作区内的文件和执行命令。

#### 默认配置

```json
{
  "agents": {
    "defaults": {
      "workspace": "~/.picoclaw/workspace",
      "restrict_to_workspace": true
    }
  }
}
```

| 选项                    | 默认值                  | 描述                          |
| ----------------------- | ----------------------- | ----------------------------- |
| `workspace`             | `~/.picoclaw/workspace` | Agent 的工作目录              |
| `restrict_to_workspace` | `true`                  | 限制文件/命令访问在工作区内   |

#### 受保护的工具

当 `restrict_to_workspace: true` 时，以下工具会被沙箱化：

| 工具          | 功能         | 限制                           |
| ------------- | ------------ | ------------------------------ |
| `read_file`   | 读取文件     | 仅限工作区内的文件             |
| `write_file`  | 写入文件     | 仅限工作区内的文件             |
| `list_dir`    | 列出目录     | 仅限工作区内的目录             |
| `edit_file`   | 编辑文件     | 仅限工作区内的文件             |
| `append_file` | 追加文件     | 仅限工作区内的文件             |
| `exec`        | 执行命令     | 命令路径必须在工作区内         |

#### 额外的 Exec 保护

即使 `restrict_to_workspace: false`，`exec` 工具也会阻止以下危险命令：

* `rm -rf`、`del /f`、`rmdir /s` — 批量删除
* `format`、`mkfs`、`diskpart` — 磁盘格式化
* `dd if=` — 磁盘镜像
* 写入 `/dev/sd[a-z]` — 直接磁盘写入
* `shutdown`、`reboot`、`poweroff` — 系统关机
* Fork bomb `:(){ :|:& };:`

### 文件访问控制

| 配置键 | 类型 | 默认值 | 描述 |
|--------|------|--------|------|
| `tools.allow_read_paths` | string[] | `[]` | 允许在工作区外读取的额外路径 |
| `tools.allow_write_paths` | string[] | `[]` | 允许在工作区外写入的额外路径 |

### Exec 安全配置

| 配置键 | 类型 | 默认值 | 描述 |
|--------|------|--------|------|
| `tools.exec.allow_remote` | bool | `false` | 允许从远程渠道（Telegram/Feishu 等）执行 exec 工具 |
| `tools.exec.enable_deny_patterns` | bool | `true` | 启用危险命令拦截 |
| `tools.exec.custom_deny_patterns` | string[] | `[]` | 自定义阻止的正则表达式模式 |
| `tools.exec.custom_allow_patterns` | string[] | `[]` | 自定义允许的正则表达式模式 |

> **安全提示：** Symlink 保护默认启用——所有文件路径在允许列表匹配前都会通过 `filepath.EvalSymlinks` 解析，防止符号链接逃逸攻击。

#### 已知限制：构建工具的子进程

exec 安全守卫仅检查 PicoClaw 直接启动的命令行。它不会递归检查由 `make`、`go run`、`cargo`、`npm run` 或自定义构建脚本等开发工具产生的子进程。

这意味着顶层命令通过初始守卫检查后，仍可以编译或启动其他二进制文件。实际上，应将构建脚本、Makefile、包脚本和生成的二进制文件视为与直接 shell 命令同等级别的可执行代码进行审查。

对于高风险环境：

* 执行前审查构建脚本。
* 对编译并运行的工作流优先使用审批/手动审查。
* 如果需要比内置守卫更强的隔离，请在容器或虚拟机中运行 PicoClaw。

#### 错误示例

```
[ERROR] tool: Tool execution failed
{tool=exec, error=Command blocked by safety guard (path outside working dir)}
```

```
[ERROR] tool: Tool execution failed
{tool=exec, error=Command blocked by safety guard (dangerous pattern detected)}
```

#### 禁用限制（安全风险）

如果需要 Agent 访问工作区外的路径：

**方法 1: 配置文件**

```json
{
  "agents": {
    "defaults": {
      "restrict_to_workspace": false
    }
  }
}
```

**方法 2: 环境变量**

```bash
export PICOCLAW_AGENTS_DEFAULTS_RESTRICT_TO_WORKSPACE=false
```

> ⚠️ **警告**: 禁用此限制将允许 Agent 访问系统上的任何路径。仅在受控环境中谨慎使用。

#### 安全边界一致性

`restrict_to_workspace` 设置在所有执行路径中一致应用：

| 执行路径         | 安全边界                     |
| ---------------- | ---------------------------- |
| 主 Agent         | `restrict_to_workspace` ✅   |
| 子 Agent / Spawn | 继承相同限制 ✅              |
| 心跳任务         | 继承相同限制 ✅              |

所有路径共享相同的工作区限制——无法通过子 Agent 或定时任务绕过安全边界。

### 心跳 / 周期性任务 (Heartbeat)

PicoClaw 可以自动执行周期性任务。在工作区创建 `HEARTBEAT.md` 文件：

```markdown
# Periodic Tasks

- Check my email for important messages
- Review my calendar for upcoming events
- Check the weather forecast
```

Agent 将每隔 30 分钟（可配置）读取此文件，并使用可用工具执行任务。

#### 使用 Spawn 的异步任务

对于耗时较长的任务（网络搜索、API 调用），使用 `spawn` 工具创建一个 **子 Agent (subagent)**：

```markdown
# Periodic Tasks

## Quick Tasks (respond directly)

- Report current time

## Long Tasks (use spawn for async)

- Search the web for AI news and summarize
- Check email and report important messages
```

**关键行为：**

| 特性             | 描述                                     |
| ---------------- | ---------------------------------------- |
| **spawn**        | 创建异步子 Agent，不阻塞主心跳进程       |
| **独立上下文**   | 子 Agent 拥有独立上下文，无会话历史      |
| **message tool** | 子 Agent 通过 message 工具直接与用户通信 |
| **非阻塞**       | spawn 后，心跳继续处理下一个任务         |

**配置：**

```json
{
  "heartbeat": {
    "enabled": true,
    "interval": 30
  }
}
```

| 选项       | 默认值 | 描述                         |
| ---------- | ------ | ---------------------------- |
| `enabled`  | `true` | 启用/禁用心跳                |
| `interval` | `30`   | 检查间隔，单位分钟 (最小: 5) |

**环境变量:**

- `PICOCLAW_HEARTBEAT_ENABLED=false` 禁用
- `PICOCLAW_HEARTBEAT_INTERVAL=60` 更改间隔

#### 子 Agent 通信流程

```
心跳触发
    ↓
Agent 读取 HEARTBEAT.md
    ↓
遇到耗时任务：spawn 子 Agent
    ↓                           ↓
继续处理下一个任务         子 Agent 独立运行
    ↓                           ↓
所有任务完成               子 Agent 使用 "message" 工具
    ↓                           ↓
回复 HEARTBEAT_OK          用户直接收到结果
```

子 Agent 拥有工具访问权限（message、web_search 等），可以独立与用户通信，无需经过主 Agent。

### 模型提供商

当前 fork 仅保留 OpenAI（Responses / Chat Completions）和 Anthropic Messages。模型配置、流式输出、登录、兼容端点、多 Key 和 fallback 说明统一见[模型接入文档](providers.zh.md)。原版的厂商专用 provider 和 CLI 代理已移除。

### 事件日志

PicoClaw 的 runtime events 会覆盖 agent、channel、gateway、message bus 和 MCP 等运行时组件。默认只打印 `agent.*` 事件，其他事件仍会发布到 runtime event bus，但不会进入日志。

```json
{
  "events": {
    "logging": {
      "enabled": true,
      "include": ["agent.*"],
      "exclude": [],
      "min_severity": "info",
      "include_payload": false
    }
  }
}
```

常用配置：

```json
{
  "events": {
    "logging": {
      "include": ["*"],
      "exclude": ["agent.llm.delta"],
      "min_severity": "warn"
    }
  }
}
```

`include` / `exclude` 支持精确事件名和 `gateway.*`、`channel.lifecycle.*` 这类模式。`include_payload` 默认关闭，避免把完整用户消息或工具参数写入日志；agent 事件会默认输出长度、计数、状态等摘要字段。

更多字段说明和示例见 [Runtime Events 与事件日志](../architecture/runtime-events.zh.md)。

### 定时任务 / 提醒

PicoClaw 通过 `cron` 工具支持 cron 风格的定时任务。Agent 可以设置、列出和取消在指定时间触发的提醒或周期性任务。

```json
{
  "tools": {
    "cron": {
      "enabled": true,
      "exec_timeout_minutes": 5
    }
  }
}
```

定时任务在重启后持久保存，存储于 `~/.picoclaw/workspace/cron/`。

### 进阶主题

| 主题 | 说明 |
| ---- | ---- |
| [敏感数据过滤](../security/sensitive_data_filtering.zh.md) | 在发送给 LLM 前，从工具结果中过滤 API 密钥和令牌 |
| [Runtime Events 与事件日志](../architecture/runtime-events.zh.md) | 统一运行时事件、日志过滤和调试配置 |
| [Hook 系统](../architecture/hooks/README.zh.md) | 事件驱动 Hook：观察者、拦截器、审批 Hook |
| [Steering](../architecture/steering.md) | 在工具调用间向运行中的 Agent 注入消息 |
| [SubTurn](../architecture/subturn.md) | 子 Agent 协调、并发控制、生命周期管理 |
| [上下文管理](../architecture/agent-refactor/context.md) | 上下文边界检测、主动预算检查、压缩策略 |
