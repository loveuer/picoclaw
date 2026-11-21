package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sipeed/picoclaw/pkg/providers"
)

var ErrSessionChanged = errors.New("session changed while the turn was running")

type SessionSnapshot struct {
	Messages []providers.Message
	RowIDs   []int64
	Summary  string
	Revision int64
	private  bool
	exists   bool
}

// ReadSession reads messages and their revision from a single SQLite read snapshot.
func (s *SQLite) ReadSession(ctx context.Context, key string) (SessionSnapshot, error) {
	var out SessionSnapshot
	rows, err := s.db.QueryContext(ctx, `
 SELECT COALESCE(s.summary,''),COALESCE(s.revision,0),COALESCE(s.private,0),s.key IS NOT NULL,m.id,m.payload
 FROM (SELECT ? AS key) AS k
 LEFT JOIN sessions s ON s.key=k.key
 LEFT JOIN session_messages m ON m.key=k.key
 ORDER BY m.id`, key)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var id sql.NullInt64
		var payload []byte
		if err = rows.Scan(&out.Summary, &out.Revision, &out.private, &out.exists, &id, &payload); err != nil {
			return SessionSnapshot{}, err
		}
		if !id.Valid {
			continue
		}
		var m providers.Message
		if err = json.Unmarshal(payload, &m); err != nil {
			return SessionSnapshot{}, err
		}
		out.Messages = append(out.Messages, m)
		out.RowIDs = append(out.RowIDs, id.Int64)
	}
	return out, rows.Err()
}

var errSessionAccess = errors.New("conversation cannot be accessed from this route")

func (s *SQLite) CheckSessionAccess(ctx context.Context, key string, private bool) error {
	var stored bool
	err := s.db.QueryRowContext(ctx, "SELECT private FROM sessions WHERE key=?", key).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err == nil && stored != private {
		return errSessionAccess
	}
	return err
}

// ReadConversation checks the access class in the same snapshot as the history.
func (s *SQLite) ReadConversation(ctx context.Context, key string, private bool) (SessionSnapshot, error) {
	snap, err := s.ReadSession(ctx, key)
	if err == nil && snap.exists && snap.private != private {
		return SessionSnapshot{}, errSessionAccess
	}
	return snap, err
}

func checkOrCreateConversation(tx *sql.Tx, key string, private bool) error {
	var stored bool
	err := tx.QueryRow("SELECT private FROM sessions WHERE key=?", key).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.Exec(`INSERT INTO sessions(key,private,created_at,updated_at)
VALUES(?,?,strftime('%Y-%m-%dT%H:%M:%fZ','now'),strftime('%Y-%m-%dT%H:%M:%fZ','now'))`, key, private)
		return err
	}
	if err == nil && stored != private {
		return errSessionAccess
	}
	return err
}

func currentRevision(tx *sql.Tx, key string) (int64, error) {
	var rev int64
	err := tx.QueryRow("SELECT revision FROM sessions WHERE key=?", key).Scan(&rev)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return rev, err
}

type SessionWrite struct {
	Key      string
	Scope    json.RawMessage
	Aliases  []string
	Private  bool
	Revision int64
	Review   *ReviewNote
}
type MessageSpan struct{ First, Last int64 }

func saveSessionMetadata(tx *sql.Tx, key string, scope json.RawMessage, aliases []string) error {
	if len(scope) > 0 {
		if _, err := tx.Exec(
			"UPDATE sessions SET scope=? WHERE key=? AND scope IS NULL",
			[]byte(scope),
			key,
		); err != nil {
			return err
		}
	}
	return insertAliases(tx, key, aliases)
}

func (s *SQLite) AppendTurn(
	ctx context.Context,
	write SessionWrite,
	messages []providers.Message,
) (MessageSpan, error) {
	var result MessageSpan
	if len(messages) == 0 {
		return result, nil
	}
	// Serialize out of the write transaction. CreatedAt is stamped by the turn
	// buffer on ingress, with a fallback for callers using this API directly.
	payloads := make([][]byte, 0, len(messages))
	for _, m := range messages {
		if m.CreatedAt == nil {
			now := time.Now().UTC()
			m.CreatedAt = &now
		}
		b, err := json.Marshal(m)
		if err != nil {
			return result, err
		}
		payloads = append(payloads, b)
	}
	err := transaction(ctx, s.db, func(tx *sql.Tx) error {
		rev, err := currentRevision(tx, write.Key)
		if err != nil {
			return err
		}
		if rev != write.Revision {
			return ErrSessionChanged
		}
		if err = checkOrCreateConversation(tx, write.Key, write.Private); err != nil {
			return err
		}
		if err = saveSessionMetadata(tx, write.Key, write.Scope, write.Aliases); err != nil {
			return err
		}
		stmt, err := tx.Prepare("INSERT INTO session_messages(key,payload) VALUES(?,?)")
		if err != nil {
			return err
		}
		defer stmt.Close()
		for i, b := range payloads {
			row, execErr := stmt.Exec(write.Key, b)
			if execErr != nil {
				return execErr
			}
			id, idErr := row.LastInsertId()
			if idErr != nil {
				return idErr
			}
			if i == 0 {
				result.First = id
			}
			result.Last = id
		}
		if write.Review != nil && write.Private {
			if err = enqueueReview(tx, write.Key, *write.Review); err != nil {
				return err
			}
		}

		return bump(tx, write.Key)
	})
	if err != nil {
		return MessageSpan{}, err
	}
	return result, nil
}

// RemoveTurnSpans rolls back only this turn's committed batches. Revision
// checking prevents a stale abort from deleting reused IDs after a clear.
func (s *SQLite) RemoveTurnSpans(ctx context.Context, write SessionWrite, spans []MessageSpan) error {
	if len(spans) == 0 {
		return nil
	}
	return transaction(ctx, s.db, func(tx *sql.Tx) error {
		rev, err := currentRevision(tx, write.Key)
		if err != nil {
			return err
		}
		if rev != write.Revision {
			return ErrSessionChanged
		}
		if err = checkOrCreateConversation(tx, write.Key, write.Private); err != nil {
			return err
		}
		for _, span := range spans {
			if _, err = tx.Exec(
				"DELETE FROM session_messages WHERE key=? AND id BETWEEN ? AND ?",
				write.Key,
				span.First,
				span.Last,
			); err != nil {
				return err
			}
		}
		return bump(tx, write.Key)
	})
}

func (s *SQLite) ClearSession(ctx context.Context, write SessionWrite) error {
	return transaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := checkOrCreateConversation(tx, write.Key, write.Private); err != nil {
			return err
		}
		if err := saveSessionMetadata(tx, write.Key, write.Scope, write.Aliases); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM session_messages WHERE key=?", write.Key); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM memory_review_notes WHERE session_key=?", write.Key); err != nil {
			return err
		}
		return updateSummaryVersion(tx, write.Key, "")
	})
}

func updateSummaryVersion(tx *sql.Tx, key, summary string) error {
	_, err := tx.Exec(
		`UPDATE sessions SET summary=?,revision=revision+1,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE key=?`,
		summary,
		key,
	)
	return err
}

// CompactPrefix applies a summary only to the exact revision it describes.
func (s *SQLite) CompactPrefix(
	ctx context.Context,
	key string,
	revision, throughID int64,
	summary string,
) (bool, error) {
	if throughID <= 0 {
		return false, fmt.Errorf("invalid session compaction boundary")
	}
	applied := false
	err := transaction(ctx, s.db, func(tx *sql.Tx) error {
		rev, err := currentRevision(tx, key)
		if err != nil {
			return err
		}
		if rev != revision {
			return nil
		}
		if _, err = tx.Exec("DELETE FROM session_messages WHERE key=? AND id<=?", key, throughID); err != nil {
			return err
		}
		if err = updateSummaryVersion(tx, key, summary); err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied && err == nil, err
}
