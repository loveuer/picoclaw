# ⚙️ Configuration Guide

> Back to [README](../../README.md)

## ⚙️ Configuration

Config file: `~/.picoclaw/config.json`

> **Security Configuration:** For storing API keys, tokens, and other sensitive data, see the [Security Configuration Guide](../security/security_configuration.md).

### Environment Variables

You can override default paths using environment variables. This is useful for portable installations, containerized deployments, or running picoclaw as a system service. `PICOCLAW_HOME` also moves the default config path; `PICOCLAW_CONFIG` takes precedence for the config file.

| Variable          | Description                                                                                                                             | Default Path              |
|-------------------|-----------------------------------------------------------------------------------------------------------------------------------------|---------------------------|
| `PICOCLAW_CONFIG` | Overrides the path to the configuration file. This directly tells picoclaw which `config.json` to load, ignoring all other locations. | `~/.picoclaw/config.json` |
| `PICOCLAW_HOME`   | Overrides the root directory for picoclaw data. This changes the default location of the `workspace` and other data directories.          | `~/.picoclaw`             |

**Examples:**

```bash
# Run picoclaw using a specific config file
# The workspace path will be read from within that config file
PICOCLAW_CONFIG=/etc/picoclaw/production.json picoclaw gateway

# Run picoclaw with all its data stored in /opt/picoclaw
# Config: /opt/picoclaw/config.json (PICOCLAW_HOME=/opt/picoclaw)
# Workspace will be created at /opt/picoclaw/workspace
PICOCLAW_HOME=/opt/picoclaw picoclaw agent

# Use both for a fully customized setup
PICOCLAW_HOME=/srv/picoclaw PICOCLAW_CONFIG=/srv/picoclaw/main.json picoclaw gateway
```

### Gateway Log Level

`gateway.log_level` controls Gateway log verbosity and is configurable in `config.json`.

```json
{
  "gateway": {
    "log_level": "warn"
  }
}
```

When omitted, the default is `warn`. Supported values: `debug`, `info`, `warn`, `error`, `fatal`.

You can also override this with the environment variable `PICOCLAW_LOG_LEVEL`.

### Workspace Layout

PicoClaw stores data in your configured workspace (default: `~/.picoclaw/workspace`):

```
~/.picoclaw/workspace/
├── sessions/          # Conversation sessions and history
├── memory/           # Long-term memory (MEMORY.md)
├── state/            # Persistent state (last channel, etc.)
├── cron/             # Scheduled jobs database
├── skills/           # Custom skills
├── AGENT.md          # Agent behavior guide
├── HEARTBEAT.md      # Periodic task prompts (checked every 30 min)
├── IDENTITY.md       # Agent identity
├── SOUL.md           # Agent soul
└── USER.md           # User preferences
```

> **Note:** Changes to `AGENT.md`, `SOUL.md`, `USER.md` and `memory/MEMORY.md` are automatically detected at runtime via file modification time (mtime) tracking. You do **not** need to restart the gateway after editing these files — the agent picks up the new content on the next request.

### Agent Self-Evolution

The `evolution` block controls PicoClaw's self-evolution runtime. When enabled, the agent records completed turns as learning records. In higher modes it can group repeated successful patterns, generate skill drafts, and optionally apply accepted drafts into workspace skills.

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

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` | Enables learning-record capture for completed agent turns. Heartbeat turns are ignored. |
| `mode` | `observe` | `observe` records data only. `draft` can generate candidate skill drafts. `apply` can apply accepted drafts to workspace skills. |
| `state_dir` | `""` | Optional directory for evolution state. Leave empty to use the default under the workspace. |
| `min_task_count` | `2` | Minimum related task records required before a pattern is eligible for draft generation. |
| `min_success_ratio` | `0.7` | Minimum success ratio for a task cluster. Use a value greater than `0` and up to `1`. |
| `cold_path_trigger` | `after_turn` | Runs draft generation `after_turn`, on a `scheduled` cadence, or disables automatic cold-path runs when set to `manual`. There is no user-facing manual trigger yet. Applies only in `draft` and `apply` modes. |
| `cold_path_times` | `[]` | Scheduled run times used when `cold_path_trigger` is `scheduled`, written as `HH:MM` strings. |

Use `observe` first if you want to inspect learning records without generating skill changes. Use `draft` when you want PicoClaw to prepare reviewable improvements. Use `apply` only when you are comfortable letting accepted drafts update workspace skills.

### Request Context Policy

`turn_profile` is an optional request context policy under `agents.defaults.turn_profile`. Leave it unset or set `"enabled": false` to keep PicoClaw's normal behavior. When `"enabled": true`, the same policy applies to every new turn.

Each block uses the same `mode` values:

| Mode | Meaning |
| --- | --- |
| `default` | Keep PicoClaw's normal behavior for that block. Missing blocks and missing `mode` fields are treated as `default`. |
| `off` | Disable that block for the turn. |
| `custom` | Use an allow list. In this version, `custom` is supported only for `skills` and `tools`; using it for `history` or `system_prompt` is a validation error. |

Profile blocks:

| Block | What it controls |
| --- | --- |
| `history` | Whether the turn reads prior session history and summary, writes user/assistant/tool messages, ingests context, and runs compaction or summarization. |
| `system_prompt` | Whether PicoClaw injects its default identity, workspace instructions, memory, runtime context, and summary. External request system prompts are still allowed when this is `off`. |
| `skills` | Whether the skill catalog and active skill prompt content are loaded. `custom.allow` keeps only the listed skill names in prompt context. |
| `tools` | Which callable tools are exposed to the model and allowed at execution time. `custom.allow` keeps only listed registered tool names. |

When `system_prompt.mode` is `off`, tools are still visible, and no external system prompt is supplied, PicoClaw uses its existing tool-use rule as the minimal fallback prompt. If `tools.mode` is `off`, no fallback prompt is added.

Example clean web policy:

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

### Skill Sources

By default, skills are loaded from:

1. `~/.picoclaw/workspace/skills` (workspace)
2. `~/.picoclaw/skills` (global)
3. `<binary-embedded-path>/skills` (builtin, set at build time)

For advanced/test setups, you can override the builtin skills root with:

```bash
export PICOCLAW_BUILTIN_SKILLS=/path/to/skills
```

### Using Skills From Chat Channels

Once skills are installed, and MCP servers are configured, you can inspect and force them directly from a chat channel:

- `/list skills` shows the installed skill names available to the current agent.
- `/list mcp` shows configured MCP servers with enabled/deferred/connected status.
- `/show mcp <server>` shows the active tools exposed by a connected MCP server.
- `/use <skill> <message>` forces a specific skill for a single request.
- `/use <skill>` arms that skill for your next message in the same chat session.
- `/use clear` cancels a pending skill override created by `/use <skill>`.
- `/btw <question>` asks an immediate side question without changing the current session history. `/btw` is handled as a no-tool query and does not enter the normal tool-execution flow.

Examples:

```text
/list skills
/list mcp
/show mcp github
/use git explain how to squash the last 3 commits
/btw remind me what we already decided about the deploy plan
/use italiapersonalfinance
dammi le ultime news
```

### Unified Command Execution Policy

- Generic slash commands are executed through a single path in `pkg/agent/loop.go` via `commands.Executor`.
- Channel adapters no longer consume generic commands locally; they forward inbound text to the bus/agent path. Telegram still auto-registers supported commands such as `/start`, `/help`, `/show`, `/list`, `/use`, and `/btw` at startup.
- Unknown slash command (for example `/foo`) passes through to normal LLM processing.
- `/new` clears the current session's history and summary, like `/clear`. `/new <prompt>` clears them and immediately runs the supplied prompt; subsequent messages continue that fresh conversation. The model, workspace, and system instructions remain active. Spaces, tabs, and newlines can separate the command from its prompt.
- If the same session already has a running or queued task, `/new` (with or without a prompt) replies with the current task's name and asks you to retry later. It does not interrupt the task or queue the new prompt. Other sessions can still use `/new`.
- Registered but unsupported command on the current channel returns an explicit user-facing error and stops further processing.

### Session Isolation

Session scope controls how much memory is shared between chats, users, threads, and spaces.

- Use `session.dimensions` for the global default.
- Use `session_dimensions` on a dispatch rule for one routed exception.

For step-by-step recipes and isolation patterns, see the [Session Guide](session-guide.md).

### Routing

Routing is configured through `agents.dispatch.rules`.

Each rule matches against the normalized inbound context produced by channels.
Rules are evaluated from top to bottom. The first matching rule wins. If no
rule matches, PicoClaw falls back to the configured default agent.

Supported match fields:

* `channel`
* `account`
* `space`
* `chat`
* `topic`
* `sender`
* `mentioned`

Match values use the same scope vocabulary as the session system:

* `space`: `workspace:t001`, `guild:123456`
* `chat`: `direct:user123`, `group:-100123`, `channel:c123`
* `topic`: `topic:42`
* `sender`: a normalized sender identifier for the platform

Rules may optionally override the global `session.dimensions` value through
`session_dimensions`. This allows routing and session allocation to stay aligned
without reintroducing the old `bindings` or `dm_scope` formats.

Example:

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

In the example above, the VIP rule must appear before the broader group rule.
Because routing is strictly ordered, more specific rules should be placed
earlier and broader fallback rules later.

For more complete routing and model-tier examples, see the [Routing Guide](routing-guide.md).

### Agent Tool Allowlist

Per-agent tool declarations live in `AGENT.md` frontmatter, not in `config.json`.

If `tools` is omitted from frontmatter, the agent gets the normal globally enabled tool set. If `tools` is present, PicoClaw registers only the listed runtime tools for that agent.

```md
---
name: Research Agent
description: Specialist for web research and in-depth analysis.
tools: [read_file, write_file, web_search, web_fetch, message]
skills: [deep-research]
mcpServers: [web-index]
---

You are the research agent.
```

Notes:

- This is an allowlist, not a preference hint.
- Tool names are matched against the runtime tool name 1:1.
- Use runtime tool names such as `web_search`, `web_fetch`, `spawn`, `subagent`, `send_file`.
- Tool declarations in `AGENT.md` are used by runtime/tooling, but they are not injected into the discovery prompt.

### Agent Discovery (Automatic)

When an agent has spawnable peers and can call `spawn`, PicoClaw injects a structured agent registry into that agent's system prompt on every turn. No extra `list_agents` tool call is required.

This registry is intended to make delegation concrete and reliable, especially when using `spawn` with a target `agent_id`.

Each entry includes:

| Field | Meaning |
|-------|---------|
| `id` | Stable agent id |
| `name` | Agent identity name from `AGENT.md` frontmatter |
| `description` | Agent identity description from `AGENT.md` frontmatter |

Important behavior:

- The discovery section appears only when the current agent has the `spawn` tool and includes only peer agents it is permitted to spawn via `subagents.allow_agents`.
- The current agent and non-spawnable peers are omitted, so the model does not plan against unavailable agents.
- Discovery is intentionally lightweight. It gives the model only the identity it needs to choose a peer: `id`, `name`, and `description`.
- `config.json` remains the infrastructure layer: workspace, default agent selection, routing, and subagent permissions. Those permissions also gate discovery visibility.
- `AGENT.md` remains the identity layer. Runtime/tool code can still use its `tools`, `skills`, `mcpServers`, and `model` fields when delegation happens.

Example injected shape:

```json
{
  "agents": [
    {
      "id": "research",
      "name": "Research Agent",
      "description": "Specialist for long-form investigation and web work."
    }
  ]
}
```

In practice, this means a generalist agent can choose a peer based on its role description, then call `spawn` with the peer's `agent_id`. The runtime resolves the rest.

### 🔒 Security Sandbox

PicoClaw runs in a sandboxed environment by default. The agent can only access files and execute commands within the configured workspace.

#### Default Configuration

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

| Option                  | Default                 | Description                               |
| ----------------------- | ----------------------- | ----------------------------------------- |
| `workspace`             | `~/.picoclaw/workspace` | Working directory for the agent           |
| `restrict_to_workspace` | `true`                  | Restrict file/command access to workspace |

#### Protected Tools

When `restrict_to_workspace: true`, the following tools are sandboxed:

| Tool          | Function         | Restriction                            |
| ------------- | ---------------- | -------------------------------------- |
| `read_file`   | Read files       | Only files within workspace            |
| `write_file`  | Write files      | Only files within workspace            |
| `list_dir`    | List directories | Only directories within workspace      |
| `edit_file`   | Edit files       | Only files within workspace            |
| `append_file` | Append to files  | Only files within workspace            |
| `exec`        | Execute commands | Command paths must be within workspace |

#### Additional Exec Protection

Even with `restrict_to_workspace: false`, the `exec` tool blocks these dangerous commands:

* `rm -rf`, `del /f`, `rmdir /s` — Bulk deletion
* `format`, `mkfs`, `diskpart` — Disk formatting
* `dd if=` — Disk imaging
* Writing to `/dev/sd[a-z]` — Direct disk writes
* `shutdown`, `reboot`, `poweroff` — System shutdown
* Fork bomb `:(){ :|:& };:`

### File Access Control

| Config Key | Type | Default | Description |
|------------|------|---------|-------------|
| `tools.allow_read_paths` | string[] | `[]` | Additional paths allowed for reading outside workspace |
| `tools.allow_write_paths` | string[] | `[]` | Additional paths allowed for writing outside workspace |
| `tools.message.media_enabled` | bool | `false` | Allows the `message` tool to attach local media files by path. This is separate from `tools.send_file.enabled`; enable it only when unified text/media/caption delivery is intended. |

### Read File Mode

`read_file` has two mutually exclusive implementations selected by config. PicoClaw registers exactly one of them at startup:

| Config Key | Type | Default | Description |
|------------|------|---------|-------------|
| `tools.read_file.enabled` | bool | `true` | Enables the `read_file` tool |
| `tools.read_file.mode` | string | `bytes` | Selects the `read_file` implementation: `bytes` or `lines` |
| `tools.read_file.max_read_file_size` | int | `65536` | Maximum bytes returned by `read_file` |

#### Mode: `bytes`

Optimized for arbitrary files and binary-safe pagination.

Parameters:

* `path` (required): File path
* `offset` (optional): Starting byte offset, default `0`
* `length` (optional): Maximum number of bytes to read, default `max_read_file_size`

Use `bytes` when:

* You may read binary files
* You want deterministic byte-range pagination

#### Mode: `lines`

Text-oriented behavior, optimized for source files, markdown, logs, and configs. The tool reads sequentially by line and stops when the configured byte budget is reached.

Parameters:

* `path` (required): File path
* `start_line` (optional): Starting line number, 1-indexed and inclusive, default `1`
* `max_lines` (optional): Maximum number of lines to read, default = all remaining lines until EOF or byte budget

Behavior notes:

* Binary-looking files are rejected with guidance to switch `read_file` to `mode = bytes`
* Extremely long single lines are truncated rather than skipped

Use `mode = lines` when:

* The agent mostly reads text files
* You want line-based pagination in prompts and tool calls
* You want cleaner chunks for code review, logs, and documentation

#### Example

```json
{
  "tools": {
    "read_file": {
      "enabled": true,
      "mode": "lines",
      "max_read_file_size": 65536
    }
  }
}
```

### Exec Security

| Config Key | Type | Default | Description |
|------------|------|---------|-------------|
| `tools.exec.allow_remote` | bool | `false` | Allow exec tool from remote channels (Telegram/Feishu etc.) |
| `tools.exec.enable_deny_patterns` | bool | `true` | Enable dangerous command interception |
| `tools.exec.custom_deny_patterns` | string[] | `[]` | Custom regex patterns to block |
| `tools.exec.custom_allow_patterns` | string[] | `[]` | Custom regex patterns to allow |

> **Security Note:** Symlink protection is enabled by default — all file paths are resolved through `filepath.EvalSymlinks` before whitelist matching, preventing symlink escape attacks.

#### Known Limitation: Child Processes From Build Tools

The exec safety guard only inspects the command line PicoClaw launches directly. It does not recursively inspect child
processes spawned by allowed developer tools such as `make`, `go run`, `cargo`, `npm run`, or custom build scripts.

That means a top-level command can still compile or launch other binaries after it passes the initial guard check. In
practice, treat build scripts, Makefiles, package scripts, and generated binaries as executable code that needs the same
level of review as a direct shell command.

For higher-risk environments:

* Review build scripts before execution.
* Prefer approval/manual review for compile-and-run workflows.
* Run PicoClaw inside a container or VM if you need stronger isolation than the built-in guard provides.

#### Error Examples

```
[ERROR] tool: Tool execution failed
{tool=exec, error=Command blocked by safety guard (path outside working dir)}
```

```
[ERROR] tool: Tool execution failed
{tool=exec, error=Command blocked by safety guard (dangerous pattern detected)}
```

#### Disabling Restrictions (Security Risk)

If you need the agent to access paths outside the workspace:

**Method 1: Config file**

```json
{
  "agents": {
    "defaults": {
      "restrict_to_workspace": false
    }
  }
}
```

**Method 2: Environment variable**

```bash
export PICOCLAW_AGENTS_DEFAULTS_RESTRICT_TO_WORKSPACE=false
```

> ⚠️ **Warning**: Disabling this restriction allows the agent to access any path on your system. Use with caution in controlled environments only.

#### Security Boundary Consistency

The `restrict_to_workspace` setting applies consistently across all execution paths:

| Execution Path   | Security Boundary            |
| ---------------- | ---------------------------- |
| Main Agent       | `restrict_to_workspace` ✅   |
| Subagent / Spawn | Inherits same restriction ✅ |
| Heartbeat tasks  | Inherits same restriction ✅ |

All paths share the same workspace restriction — there's no way to bypass the security boundary through subagents or scheduled tasks.

### Heartbeat (Periodic Tasks)

PicoClaw can perform periodic tasks automatically. Create a `HEARTBEAT.md` file in your workspace:

```markdown
# Periodic Tasks

- Check my email for important messages
- Review my calendar for upcoming events
- Check the weather forecast
```

The agent will read this file every 30 minutes (configurable) and execute any tasks using available tools.

#### Async Tasks with Spawn

For long-running tasks (web search, API calls), use the `spawn` tool to create a **subagent**:

```markdown
# Periodic Tasks

## Quick Tasks (respond directly)

- Report current time

## Long Tasks (use spawn for async)

- Search the web for AI news and summarize
- Check email and report important messages
```

**Key behaviors:**

| Feature                 | Description                                               |
| ----------------------- | --------------------------------------------------------- |
| **spawn**               | Creates async subagent, doesn't block heartbeat           |
| **Independent context** | Subagent has its own context, no session history          |
| **message tool**        | Subagent communicates with user directly via message tool |
| **Non-blocking**        | After spawning, heartbeat continues to next task          |

#### How Subagent Communication Works

```
Heartbeat triggers
    ↓
Agent reads HEARTBEAT.md
    ↓
For long task: spawn subagent
    ↓                           ↓
Continue to next task      Subagent works independently
    ↓                           ↓
All tasks done            Subagent uses "message" tool
    ↓                           ↓
Respond HEARTBEAT_OK      User receives result directly
```

The subagent has access to tools (message, web_search, etc.) and can communicate with the user independently without going through the main agent.

**Configuration:**

```json
{
  "heartbeat": {
    "enabled": true,
    "interval": 30
  }
}
```

| Option     | Default | Description                        |
| ---------- | ------- | ---------------------------------- |
| `enabled`  | `true`  | Enable/disable heartbeat           |
| `interval` | `30`    | Check interval in minutes (min: 5) |

**Environment variables:**

* `PICOCLAW_HEARTBEAT_ENABLED=false` to disable
* `PICOCLAW_HEARTBEAT_INTERVAL=60` to change interval

### Model providers

This fork supports OpenAI (Responses / Chat Completions) and Anthropic Messages. See the [provider guide](providers.md) for model configuration, streaming, authentication, compatible endpoints, multiple keys and fallback. The upstream named-provider catalog and CLI proxies have been removed.

### Scheduled Tasks / Reminders

PicoClaw supports cron-style scheduled tasks via the `cron` tool. The agent can set, list, and cancel reminders or recurring jobs that trigger at specified times.

```json
{
  "tools": {
    "cron": {
      "enabled": true,
      "exec_timeout_minutes": 5,
      "allow_command": true,
      "command_allowed_remotes": []
    }
  }
}
```

Scheduled tasks persist across restarts and are stored in `~/.picoclaw/workspace/cron/`.

Command cron jobs can execute shell commands. By default, remote channels cannot schedule command jobs. To allow specific remote channels, set `command_allowed_remotes` to entries such as `"telegram"` or `"telegram:1234567890"`; use `"*"` only if every non-empty channel should be allowed. The `"*"` wildcard is potentially dangerous because any remote channel that can talk to PicoClaw can schedule shell commands. This does not bypass `allow_command`, `command_confirm`, or exec safety checks.

### Advanced Topics

| Topic | Description |
| ----- | ----------- |
| [Security Configuration](../security/security_configuration.md) | Store API keys and secrets in separate `.security.yml` file |
| [Sensitive Data Filtering](../security/sensitive_data_filtering.md) | Filter API keys and tokens from tool results before sending to LLM |
| [Hook System](../architecture/hooks/README.md) | Event-driven hooks: observers, interceptors, approval hooks |
| [Steering](../architecture/steering.md) | Inject messages into a running agent loop between tool calls |
| [SubTurn](../architecture/subturn.md) | Subagent coordination, concurrency control, lifecycle |
| [Context Management](../architecture/agent-refactor/context.md) | Context boundary detection, proactive budget check, compression |
