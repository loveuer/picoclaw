package memory

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/pkg/providers"
)

func TestCompactPrefixPreservesRowsAndReusesPages(t *testing.T) {
	path := Path(t.TempDir())
	db, err := Open(path)
	require.NoError(t, err)
	defer db.Close()
	ctx := t.Context()
	msgs := make([]providers.Message, 0, 60)
	for i := range 60 {
		role := "assistant"
		if i%2 == 0 {
			role = "user"
		}
		msgs = append(msgs, providers.Message{Role: role, Content: strings.Repeat("a useful note. ", 650)})
	}
	_, err = db.AppendTurn(ctx, SessionWrite{Key: "session"}, msgs)
	require.NoError(t, err)
	old, err := db.ReadSession(ctx, "session")
	require.NoError(t, err)
	size, err := os.Stat(path)
	require.NoError(t, err)
	ok, err := db.CompactPrefix(ctx, "session", old.Revision, old.RowIDs[43], "summary")
	require.NoError(t, err)
	require.True(t, ok)
	after, err := db.ReadSession(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, old.RowIDs[44:], after.RowIDs)
	require.Equal(t, old.Messages[44:], after.Messages)
	require.Equal(t, "summary", after.Summary)
	var free int
	require.NoError(t, db.db.QueryRow("PRAGMA freelist_count").Scan(&free))
	require.Positive(t, free)
	_, err = db.AppendTurn(ctx, SessionWrite{Key: "session", Revision: after.Revision}, msgs[:20])
	require.NoError(t, err)
	now, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, size.Size(), now.Size(), "deleted pages should be reused")
	require.NoError(t, db.ClearSession(ctx, SessionWrite{Key: "session"}))
	ok, err = db.CompactPrefix(ctx, "session", after.Revision, after.RowIDs[0], "stale")
	require.NoError(t, err)
	require.False(t, ok)
	empty, err := db.ReadSession(ctx, "session")
	require.NoError(t, err)
	require.Empty(t, empty.Messages)
	require.Empty(t, empty.Summary)
}

func TestUnchangedSessionMetadataDoesNotWriteRows(t *testing.T) {
	db, err := Open(Path(t.TempDir()))
	require.NoError(t, err)
	defer db.Close()
	ctx := t.Context()
	scope := json.RawMessage(`{"channel":"telegram"}`)
	require.NoError(t, db.UpsertSessionMeta(ctx, "session", scope, []string{"old"}))
	var old, now int
	require.NoError(t, db.db.QueryRow("SELECT total_changes()").Scan(&old))
	require.NoError(t, db.UpsertSessionMeta(ctx, "session", scope, []string{"old"}))
	require.NoError(t, db.db.QueryRow("SELECT total_changes()").Scan(&now))
	require.Equal(t, old, now)
}
