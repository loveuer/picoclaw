# SQLite memory

This fork uses a single database at `$PICOCLAW_HOME/state/memory/memory.sqlite3` (by default, `~/.picoclaw/state/memory/memory.sqlite3`). Each `PICOCLAW_HOME` has one memory store. A workspace holds files and skills; changing it does not change the memory store. Keep the database and its directory on a persistent local filesystem, outside tool workspaces.

| Layer | Scope | Loading and removal |
| --- | --- | --- |
| Recent messages and summaries | Routed conversation | Loaded for that conversation within the model's token budget. `/new` and `/clear` clear this layer; `/new <prompt>` clears it before running the prompt. |
| Important preferences | Instance | Included in each authorized prompt, up to 12 entries of 160 characters and 500 tokens in total. Update or delete by key. |
| Dated records | Instance | Loaded only via `memory_search`. Explicit writes and high-confidence automatic capture; records do not expire. Update, delete and export by ID. |

If a private chat is busy, retry after the current turn; queued messages are not allowed to inherit private authorization.

The local `picoclaw agent` command and Feishu or Weixin direct messages permitted by the existing channel rules share preferences and records. Configure `allow_from` for the senders you trust with the whole store: an open allowlist makes the store accessible to anyone admitted to a direct chat. Groups and synthetic turns have no access. Recent conversations and summaries stay separate.

Use plain authored text, not quoted, forwarded or attachment text. For example:

```text
From now on use Chinese and keep replies brief.
帮我记一下，2026-05-21 原神打到第三章
Where did I leave off in Genshin?
原神进度改成第四章
删掉上次记的塞尔达宝箱清单
Remove my brief reply style preference.
```

Direct saves such as “帮我记一下…” and “记录一下…” commit immediately without waiting for a model. For other explicit management requests the Agent uses `memory_write` and confirms only after commit. It uses `memory_search` first when correcting or deleting a record. A correction preserves the record ID, source and any date the user did not change. A complete directive that names the record ID and both values can apply immediately if the old value occurs once. Requests that combine a preference with another task stay in the normal conversation flow. For an ambiguous replacement, review the exact before and after text, then reply “确认更正” in the same conversation. If more than one result could match, reply in the same conversation, for example “删第二条” or “11 月 21 日那条”. Choices and correction previews expire after five minutes. `/new` and `/clear` discard them.

After explicit deletion, notes already queued cannot restore the deleted item. For preferences this applies to the deleted key; after deleting a record old notes can still yield preferences, but cannot produce records. New notes are reviewed as usual. Ordinary conversation continues through scheduled review.

Explicit saves accept an ISO date; otherwise the user's preferred timezone or the Gateway timezone supplies the date. Search supports aliases, tags, date filters and short Chinese substrings and returns at most five excerpts with sources. Existing `/memory` and `/preference` commands remain available for compatibility; `/memory export` returns one JSON record at a time.

A record can carry a category path, such as `/memory add {"text":"原神玩到第三章","category":"日常/游戏"}`. The optional `records.category` column stores one path per record. A parent category such as `日常` includes `日常/游戏`. Use `tags` for subject words such as `宝箱` and `神庙`, and `aliases` for alternate names such as `旷野之息` and `BOTW`. Immediate saves use a supplied category or a confident local match without calling a classifier. Otherwise the record is committed as `other` and the receipt returns immediately. A single model call then tries to fill in the category, tags and aliases on that same record. If it is uncertain, fails, times out, or the process exits first, the record stays in `other` and remains searchable. This does not use the scheduled review queue, and CLI exit does not wait. If the record was edited or deleted during classification, the result is discarded.

Search can try several independent queries via `alternatives`. Results prioritize the primary query, then combine and deduplicate alternate matches. Date and explicitly requested category filters apply to every query. Mentioning a game alone does not narrow the category. For example, the model can pass `箱子` and `神庙` as alternatives when searching for `宝箱`.

## Scheduled review

Gateway checks for new, directly authored private messages every thirty seconds. It reviews a changed conversation after two idle minutes, or when the oldest pending note reaches ten minutes in a busy conversation. It sends at most twenty new messages per batch and keeps the latest two hundred pending messages per conversation. The CLI can enqueue messages for Gateway in the same home to review, even after the CLI exits. Both paths update the owner’s memory. Groups, quoted messages and attachments are excluded.

The background review applies supported changes without confirmation. It upserts preferences by key, limiting new preferences to twelve per person, and updates rolling records by topic while retaining their IDs. Distinct dated events can have separate records. Preferences in the prompt are subject to the same entry limit and a token budget. A qualifying Feishu or Weixin private chat receives a count of changes after commit. Set `memory.disable_auto_capture` to `true` to disable background review while keeping explicit saves. `/new` and `/clear` discard pending notes from that conversation, while already saved private memory stays. Review starts with new turns after the upgrade; old chats are not replayed.

## Initialization and persistence

PicoClaw creates a SQLite v1 database on first startup. It ignores old JSONL, Seahorse and Markdown memory files in the workspace. Backup and restore support SQLite v1 databases with the current schema.

Recent messages are committed atomically at the end of an ordinary turn. Long turns also commit after a complete tool batch once eight batches or 512 KiB of pending messages accumulate. After a crash, only committed batches survive. Private preference and record mutations and conversation resets still commit immediately. A context-limit retry trims the prompt only. Asynchronous summarization deletes the older message prefix and updates its summary and revision without rewriting retained rows.

SQLite reuses free pages; deletion does not necessarily make the file smaller. The request path does not run `VACUUM` or change journal and synchronous settings. Use the backup and offline restore workflow when the file needs to be compacted. If WAL is enabled in a future version, monitor its size and checkpoint progress; avoid forcing truncation on each turn.

## Backup and recovery

```bash
picoclaw memory backup /secure-archive/picoclaw-memory.sqlite3
# Stop Gateway and CLI agents before restoring:
picoclaw memory restore /secure-archive/picoclaw-memory.sqlite3
```

Backup creates an SQLite snapshot without overwriting an existing file. Backup and restore support stable v1, including pending notes and record topics. Restoring an older snapshot rolls back later changes. Restore validates the snapshot and saves a copy of the previous database before replacing it. Use the same `PICOCLAW_HOME` as Gateway, keep backup files private, and do not copy a live database and its journal independently. Back up config, media and workspace resources separately if you need them.
