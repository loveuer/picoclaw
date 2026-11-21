package session

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"

	"github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// MetadataAwareSessionStore exposes structured session metadata operations.
type MetadataAwareSessionStore interface {
	EnsureSessionMetadata(sessionKey string, scope *SessionScope, aliases []string)
	ResolveSessionKey(sessionKey string) string
	GetSessionScope(sessionKey string) *SessionScope
}

// SQLiteSessionStore exposes transactional session operations to the context manager.
type SQLiteSessionStore struct {
	sqlite      *memory.SQLite
	mu          sync.Mutex
	writeErrors map[string]error
}

func NewSQLiteSessionStore(store *memory.SQLite) *SQLiteSessionStore {
	return &SQLiteSessionStore{
		sqlite:      store,
		writeErrors: make(map[string]error),
	}
}

func (b *SQLiteSessionStore) resolveSessionKey(sessionKey string) string {
	resolved, found, err := b.sqlite.ResolveSessionKey(context.Background(), sessionKey)
	if err != nil {
		log.Printf("session: resolve session key: %v", err)
		return sessionKey
	}
	if found && resolved != "" {
		return resolved
	}
	return sessionKey
}

// ResolveSessionKey maps known aliases to canonical keys.
func (b *SQLiteSessionStore) ResolveSessionKey(sessionKey string) string {
	return b.resolveSessionKey(sessionKey)
}

// EnsureSessionMetadata persists scope and alias metadata for a session.
func (b *SQLiteSessionStore) EnsureSessionMetadata(sessionKey string, scope *SessionScope, aliases []string) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return
	}

	var rawScope json.RawMessage
	if scope != nil {
		data, err := json.Marshal(scope)
		if err != nil {
			log.Printf("session: encode session scope: %v", err)
			return
		}
		rawScope = data
	}
	ctx := context.Background()
	if err := b.sqlite.UpsertSessionMeta(ctx, sessionKey, rawScope, aliases); err != nil {
		log.Printf("session: upsert session metadata: %v", err)
	}
}

// GetSessionScope reads structured scope metadata for a session key or alias.
func (b *SQLiteSessionStore) GetSessionScope(sessionKey string) *SessionScope {
	sessionKey = b.resolveSessionKey(sessionKey)
	meta, err := b.sqlite.GetSessionMeta(context.Background(), sessionKey)
	if err != nil {
		log.Printf("session: get session metadata: %v", err)
		return nil
	}
	if len(meta.Scope) == 0 {
		return nil
	}
	var scope SessionScope
	if err := json.Unmarshal(meta.Scope, &scope); err != nil {
		log.Printf("session: decode session scope: %v", err)
		return nil
	}
	return CloneScope(&scope)
}

func (b *SQLiteSessionStore) GetHistory(key string) []providers.Message {
	key = b.resolveSessionKey(key)
	msgs, err := b.sqlite.GetHistory(context.Background(), key)
	if err != nil {
		log.Printf("session: get history: %v", err)
		return []providers.Message{}
	}
	return msgs
}

func (b *SQLiteSessionStore) GetSummary(key string) string {
	key = b.resolveSessionKey(key)
	summary, err := b.sqlite.GetSummary(context.Background(), key)
	if err != nil {
		log.Printf("session: get summary: %v", err)
		return ""
	}
	return summary
}

// Close releases resources held by the underlying store.
func (b *SQLiteSessionStore) Close() error {
	return b.sqlite.Close()
}

// ListSessions returns all known session keys.
func (b *SQLiteSessionStore) ListSessions() []string {
	return b.sqlite.ListSessions()
}

func (b *SQLiteSessionStore) Snapshot(ctx context.Context, key string) ([]providers.Message, string, int64, error) {
	k, found, err := b.sqlite.ResolveSessionKey(ctx, key)
	if err != nil {
		return nil, "", 0, err
	}
	if !found {
		k = key
	}
	return b.sqlite.Snapshot(ctx, k)
}

func (b *SQLiteSessionStore) Replace(
	ctx context.Context,
	key string,
	history []providers.Message,
	summary string,
	revision *int64,
) (bool, error) {
	k, found, err := b.sqlite.ResolveSessionKey(ctx, key)
	if err != nil {
		return false, err
	}
	if !found {
		k = key
	}
	return b.sqlite.Replace(ctx, k, history, summary, revision)
}

// Record write failures until Save can report them to the calling turn.
func (b *SQLiteSessionStore) remember(key string, err error) {
	if err == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.writeErrors[key] = errors.Join(b.writeErrors[key], err)
}

func (b *SQLiteSessionStore) Save(key string) error {
	key = b.resolveSessionKey(key)
	b.mu.Lock()
	defer b.mu.Unlock()
	err := b.writeErrors[key]
	delete(b.writeErrors, key)
	return err
}

func (b *SQLiteSessionStore) AddMessage(key, role, text string) {
	key = b.resolveSessionKey(key)
	b.remember(key, b.sqlite.AddMessage(context.Background(), key, role, text))
}

func (b *SQLiteSessionStore) AddFullMessage(key string, msg providers.Message) {
	key = b.resolveSessionKey(key)
	b.remember(key, b.sqlite.AddFullMessage(context.Background(), key, msg))
}

func (b *SQLiteSessionStore) SetHistory(key string, history []providers.Message) {
	key = b.resolveSessionKey(key)
	b.remember(key, b.sqlite.SetHistory(context.Background(), key, history))
}

func (b *SQLiteSessionStore) SetSummary(key, summary string) {
	key = b.resolveSessionKey(key)
	b.remember(key, b.sqlite.SetSummary(context.Background(), key, summary))
}

func (b *SQLiteSessionStore) TruncateHistory(key string, n int) {
	key = b.resolveSessionKey(key)
	b.remember(key, b.sqlite.TruncateHistory(context.Background(), key, n))
}

// TurnSnapshot pins the resolved key and revision for the whole turn.
func (b *SQLiteSessionStore) TurnSnapshot(
	ctx context.Context,
	key string,
	private bool,
) (string, memory.SessionSnapshot, error) {
	resolved, found, err := b.sqlite.ResolveSessionKey(ctx, key)
	if err != nil {
		return "", memory.SessionSnapshot{}, err
	}
	if !found {
		resolved = key
	}
	if resolved != key && private {
		return "", memory.SessionSnapshot{}, errors.New("aliases cannot authorize private memory")
	}
	snap, err := b.sqlite.ReadConversation(ctx, resolved, private)
	return resolved, snap, err
}

func (b *SQLiteSessionStore) ReadSession(ctx context.Context, key string) (memory.SessionSnapshot, error) {
	resolved, found, err := b.sqlite.ResolveSessionKey(ctx, key)
	if err != nil {
		return memory.SessionSnapshot{}, err
	}
	if !found {
		resolved = key
	}
	return b.sqlite.ReadSession(ctx, resolved)
}

func (b *SQLiteSessionStore) AppendTurn(
	ctx context.Context,
	write memory.SessionWrite,
	msgs []providers.Message,
) (memory.MessageSpan, error) {
	return b.sqlite.AppendTurn(ctx, write, msgs)
}

func (b *SQLiteSessionStore) RemoveTurnSpans(
	ctx context.Context,
	write memory.SessionWrite,
	spans []memory.MessageSpan,
) error {
	return b.sqlite.RemoveTurnSpans(ctx, write, spans)
}

func (b *SQLiteSessionStore) ClearSession(ctx context.Context, write memory.SessionWrite) error {
	resolved, found, err := b.sqlite.ResolveSessionKey(ctx, write.Key)
	if err != nil {
		return err
	}
	if found && resolved != write.Key {
		if write.Private {
			return errors.New("aliases cannot authorize private memory")
		}
		write.Key = resolved
	}
	return b.sqlite.ClearSession(ctx, write)
}

func (b *SQLiteSessionStore) CompactPrefix(
	ctx context.Context,
	key string,
	rev, through int64,
	summary string,
) (bool, error) {
	resolved, found, err := b.sqlite.ResolveSessionKey(ctx, key)
	if err != nil {
		return false, err
	}
	if !found {
		resolved = key
	}
	return b.sqlite.CompactPrefix(ctx, resolved, rev, through, summary)
}
