# Session System

> Back to [README](../../README.md)

This document describes the runtime session system used by PicoClaw to:

- map inbound messages onto stable conversation scopes
- persist message history and summaries
- preserve compatibility with legacy `agent:...` session keys while the runtime uses opaque canonical keys

This document covers the core runtime path in `pkg/session`, `pkg/memory`, and `pkg/agent`.

## Responsibilities

The session system has four jobs:

1. Decide which messages should share the same conversation context.
2. Persist that context durably across turns and restarts.
3. Expose a small `SessionStore` interface to the agent loop.
4. Resolve session aliases used by routing and tools.

## Main Components

| Layer | Files | Responsibility |
| --- | --- | --- |
| Session contract | `pkg/session/session_store.go` | Defines the `SessionStore` interface used by the agent loop. |
| Session adapter | `pkg/session/sqlite_backend.go` | Implements `SessionStore` and handles aliases and scopes. |
| Durable storage | `pkg/memory/sqlite.go` | Stores sessions, messages and aliases in SQLite. |
| Scope and key building | `pkg/session/scope.go`, `pkg/session/key.go`, `pkg/session/allocator.go` | Builds structured scopes, opaque canonical keys, and legacy aliases from routing results. |
| Runtime integration | `pkg/agent/instance.go`, `pkg/agent/agent.go`, `pkg/agent/agent_message.go` | Initializes the store, allocates session scope, and persists metadata before turns run. |

## Session Data Model

The structured session identity is represented by `session.SessionScope`:

| Field | Meaning |
| --- | --- |
| `Version` | Schema version. Current value is `ScopeVersionV1`. |
| `AgentID` | Routed agent handling the turn. |
| `Channel` | Normalized inbound channel name. |
| `Account` | Normalized account or bot identifier. |
| `Dimensions` | Ordered list of active partition dimensions such as `chat` or `sender`. |
| `Values` | Concrete normalized values for each selected dimension. |

Only four dimensions are currently recognized by the allocator:

- `space`
- `chat`
- `topic`
- `sender`

The default config uses:

```json
{
  "session": {
    "dimensions": ["chat"]
  }
}
```

That means one shared conversation per chat unless a dispatch rule overrides it.

## Canonical Keys And Legacy Aliases

The runtime now prefers opaque canonical keys:

```text
sk_v1_<sha256>
```

These keys are built from a canonical scope signature in `pkg/session/key.go`.
The goal is to make storage keys stable while decoupling them from any specific legacy text format.

For compatibility, the allocator also emits legacy aliases such as:

```text
agent:main:direct:user123
agent:main:feishu:channel:c001
agent:main:pico:direct:pico:session-123
```

Some tools and tests use these aliases.
The SQLite session store resolves aliases back to the canonical key before reads and writes.

The agent loop also preserves explicit incoming session keys when the caller already supplied one of the recognized explicit formats:

- opaque canonical key
- legacy `agent:...` key

That behavior lives in `pkg/agent/agent_utils.go:resolveScopeKey`.

## Allocation Flow

The end-to-end flow for a normal inbound message is:

```text
InboundMessage
  -> RouteResolver.ResolveRoute(...)
  -> session.AllocateRouteSession(...)
  -> resolveScopeKey(...)
  -> ensureSessionMetadata(...)
  -> AgentLoop turn execution
  -> SessionStore read/write operations
```

More concretely:

1. `pkg/agent/agent_message.go` resolves the agent route from normalized inbound context.
2. `session.AllocateRouteSession` converts the route's `SessionPolicy` plus inbound context into a structured `SessionScope`.
3. The allocator builds:
   - `SessionKey`: canonical routed session key
   - `SessionAliases`: compatibility aliases for that routed scope
   - `MainSessionKey`: agent-level main session key
   - `MainAliases`: legacy alias for the main session
4. `runAgentLoop` persists scope metadata and aliases through `ensureSessionMetadata`.
5. During later reads or writes, `SQLiteSessionStore.ResolveSessionKey` maps aliases back onto the canonical key.

The main session key is separate from routed chat sessions.
It is mainly used for agent-level or system-style flows that need one stable per-agent conversation, for example `processSystemMessage`.

## Scope Construction Rules

`pkg/session/allocator.go` builds scope values from normalized inbound context.
Important rules:

- `space` becomes `<space_type>:<space_id>`
- `chat` becomes `<chat_type>:<chat_id>`
- `topic` becomes `topic:<topic_id>`
- `sender` is canonicalized through `session.identity_links` before being stored

There are two special cases worth calling out.

### Telegram forum isolation

Telegram forum topics must stay isolated even when the configured dimensions only mention `chat`.
To preserve that behavior, the allocator appends `/<topic_id>` to the `chat` value for Telegram forum messages unless `topic` is already an explicit dimension.

Example:

```text
group:-1001234567890/42
group:-1001234567890/99
```

Those produce different session keys.

### Identity links

`session.identity_links` lets multiple sender identifiers collapse into one canonical identity.
Both dispatch matching and session allocation use that mapping so that the same person can keep one conversation even if their raw sender IDs differ across channels or accounts.

## Storage and commits

SQLite is the runtime session store. Each `PICOCLAW_HOME` has one database. The `sessions` table stores summaries, scopes and revisions, `session_messages` stores messages, and `session_aliases` stores routing aliases.

Ordinary turns commit at the end. Long turns can commit after a complete tool batch, and committed messages survive process exit. Summary compaction and conversation clearing use session revisions so stale writes cannot restore cleared history.

## Other SessionStore Implementations

`pkg/agent/subturn.go` defines an `ephemeralSessionStore`.
It satisfies the same `SessionStore` interface, but keeps data in memory only and is destroyed when the sub-turn ends.

That lets SubTurn reuse the same session-facing APIs without writing child-session history into the parent's durable storage.

## Operational Consumers

The session system is consumed by more than the agent loop:

- `pkg/agent/steering.go` can recover scope metadata for active steering flows.
- tooling and tests can still refer to legacy aliases because alias resolution is handled below the agent loop.

## Related Files

- `pkg/session/session_store.go`
- `pkg/session/manager.go`
- `pkg/session/sqlite_backend.go`
- `pkg/session/scope.go`
- `pkg/session/key.go`
- `pkg/session/allocator.go`
- `pkg/memory/sqlite.go`
- `pkg/agent/instance.go`
- `pkg/agent/agent.go`
- `pkg/agent/agent_message.go`
