package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sipeed/picoclaw/pkg/providers"
)

type SQLite struct{ db *sql.DB }

func Path(home string) string { return filepath.Join(home, "state", "memory", "memory.sqlite3") }

const schema = `
CREATE TABLE sessions (key TEXT PRIMARY KEY, summary TEXT NOT NULL DEFAULT '', scope BLOB, revision INTEGER NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '', private INTEGER NOT NULL DEFAULT 0);
CREATE TABLE session_aliases (alias TEXT PRIMARY KEY, key TEXT NOT NULL REFERENCES sessions(key) ON DELETE CASCADE);
CREATE TABLE session_messages (id INTEGER PRIMARY KEY, key TEXT NOT NULL REFERENCES sessions(key) ON DELETE CASCADE, payload BLOB NOT NULL);
CREATE INDEX session_messages_key ON session_messages(key,id);
CREATE TABLE preferences (key TEXT PRIMARY KEY, value TEXT NOT NULL, source TEXT NOT NULL, updated_at TEXT NOT NULL, memory_evidence INTEGER);
CREATE TABLE records (id INTEGER PRIMARY KEY AUTOINCREMENT, body TEXT NOT NULL, event_date TEXT NOT NULL, category TEXT, aliases TEXT NOT NULL, tags TEXT NOT NULL, source TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, memory_topic TEXT, memory_evidence INTEGER);
CREATE INDEX records_date ON records(event_date,id);
CREATE INDEX records_category ON records(category);
CREATE UNIQUE INDEX records_memory_topic ON records(memory_topic) WHERE memory_topic IS NOT NULL;
CREATE VIRTUAL TABLE records_fts USING fts5(body, aliases, tags, content='records', content_rowid='id');
CREATE TRIGGER records_ai AFTER INSERT ON records BEGIN INSERT INTO records_fts(rowid,body,aliases,tags) VALUES(new.id,new.body,new.aliases,new.tags); END;
CREATE TRIGGER records_ad AFTER DELETE ON records BEGIN INSERT INTO records_fts(records_fts,rowid,body,aliases,tags) VALUES('delete',old.id,old.body,old.aliases,old.tags); END;
CREATE TRIGGER records_au AFTER UPDATE OF body,aliases,tags ON records
 WHEN old.body IS NOT new.body OR old.aliases IS NOT new.aliases OR old.tags IS NOT new.tags BEGIN
 INSERT INTO records_fts(records_fts,rowid,body,aliases,tags) VALUES('delete',old.id,old.body,old.aliases,old.tags);
 INSERT INTO records_fts(rowid,body,aliases,tags) VALUES(new.id,new.body,new.aliases,new.tags);
END;
CREATE TABLE memory_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO memory_meta VALUES ('schema','single-home-v1');
CREATE TABLE memory_review_notes (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 session_key TEXT NOT NULL REFERENCES sessions(key) ON DELETE CASCADE,
 created INTEGER NOT NULL, body TEXT NOT NULL, source TEXT NOT NULL,
 channel TEXT NOT NULL, chat_id TEXT NOT NULL, sender TEXT NOT NULL
);
CREATE INDEX memory_review_session ON memory_review_notes(session_key,id);
PRAGMA user_version=1;`

const schemaVersion = 1

func sqliteURI(path, query string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: query}).String()
}

func openWritable(path string) (*sql.DB, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err = os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	err = f.Chmod(0o600)
	err = errors.Join(err, f.Close())
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(
		"sqlite",
		sqliteURI(path, "_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_txlock=immediate"),
	)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

func Open(path string) (*SQLite, error) {
	db, err := openWritable(path)
	if err != nil {
		return nil, err
	}
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err == nil {
		if version == 0 {
			err = transaction(context.Background(), db, initializeSchema)
		} else {
			err = checkSchema(context.Background(), db, version)
		}
	}
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open memory database: %w", err)
	}
	return &SQLite{db: db}, nil
}

func checkSchema(ctx context.Context, db interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}, version int,
) error {
	if version != schemaVersion {
		return fmt.Errorf(
			"unsupported memory database version %d; retain the old database and use a separate v1 database",
			version,
		)
	}
	var format string
	err := db.QueryRowContext(ctx, "SELECT value FROM memory_meta WHERE key='schema'").Scan(&format)
	if err != nil || format != "single-home-v1" {
		return errors.New("unrecognized memory database format; retain the old database and use a separate v1 database")
	}
	var hasCategory bool
	err = db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pragma_table_info('records') WHERE name='category')").Scan(&hasCategory)
	if err != nil {
		return err
	}
	if !hasCategory {
		return errors.New("memory database is an older v1 format without records.category; keep it and initialize a separate database")
	}
	return nil
}

func initializeSchema(tx *sql.Tx) error {
	var version, objects int
	if err := tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version != 0 {
		return checkSchema(context.Background(), tx, version)
	}
	if err := tx.QueryRow("SELECT count(*) FROM sqlite_schema").Scan(&objects); err != nil {
		return err
	}
	if objects != 0 {
		return errors.New("refusing to initialize a nonempty unversioned database")
	}
	_, err := tx.Exec(schema)
	return err
}

func (s *SQLite) Close() error { return s.db.Close() }
func transaction(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func ensureSession(tx *sql.Tx, key string) error {
	_, err := tx.Exec(
		`INSERT INTO sessions(key,created_at,updated_at) VALUES(?,strftime('%Y-%m-%dT%H:%M:%fZ','now'),strftime('%Y-%m-%dT%H:%M:%fZ','now')) ON CONFLICT DO NOTHING`,
		key,
	)
	return err
}

func bump(tx *sql.Tx, key string) error {
	_, err := tx.Exec(
		`UPDATE sessions SET revision=revision+1,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE key=?`,
		key,
	)
	return err
}

func appendMessage(tx *sql.Tx, key string, m providers.Message) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT INTO session_messages(key,payload) VALUES(?,?)", key, b)
	return err
}

func (s *SQLite) AddMessage(ctx context.Context, k, r, c string) error {
	return s.AddFullMessage(ctx, k, providers.Message{Role: r, Content: c})
}

func (s *SQLite) AddFullMessage(ctx context.Context, k string, m providers.Message) error {
	if m.CreatedAt == nil {
		now := time.Now().UTC()
		m.CreatedAt = &now
	}
	return transaction(ctx, s.db, func(tx *sql.Tx) error {
		if e := ensureSession(tx, k); e != nil {
			return e
		}
		if e := appendMessage(tx, k, m); e != nil {
			return e
		}
		return bump(tx, k)
	})
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func history(ctx context.Context, q queryer, k string) ([]providers.Message, error) {
	rows, e := q.QueryContext(ctx, "SELECT payload FROM session_messages WHERE key=? ORDER BY id", k)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []providers.Message{}
	for rows.Next() {
		var b []byte
		var m providers.Message
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &m); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *SQLite) Snapshot(
	ctx context.Context,
	k string,
) (out []providers.Message, summary string, rev int64, err error) {
	snap, err := s.ReadSession(ctx, k)
	return snap.Messages, snap.Summary, snap.Revision, err
}

func (s *SQLite) GetHistory(ctx context.Context, k string) ([]providers.Message, error) {
	return history(ctx, s.db, k)
}

func (s *SQLite) GetSummary(ctx context.Context, k string) (string, error) {
	var v string
	e := s.db.QueryRowContext(ctx, "SELECT summary FROM sessions WHERE key=?", k).Scan(&v)
	if errors.Is(e, sql.ErrNoRows) {
		e = nil
	}
	return v, e
}

func (s *SQLite) SetSummary(ctx context.Context, k, v string) error {
	return transaction(ctx, s.db, func(tx *sql.Tx) error {
		if e := ensureSession(tx, k); e != nil {
			return e
		}
		if _, e := tx.Exec("UPDATE sessions SET summary=? WHERE key=?", v, k); e != nil {
			return e
		}
		return bump(tx, k)
	})
}

func (s *SQLite) SetHistory(ctx context.Context, k string, msgs []providers.Message) error {
	return transaction(ctx, s.db, func(tx *sql.Tx) error { return replaceSession(tx, k, msgs, nil) })
}

func replaceSession(tx *sql.Tx, k string, msgs []providers.Message, summary *string) error {
	if e := ensureSession(tx, k); e != nil {
		return e
	}
	if _, e := tx.Exec("DELETE FROM session_messages WHERE key=?", k); e != nil {
		return e
	}
	for _, m := range msgs {
		if e := appendMessage(tx, k, m); e != nil {
			return e
		}
	}
	if summary != nil {
		if _, e := tx.Exec("UPDATE sessions SET summary=? WHERE key=?", *summary, k); e != nil {
			return e
		}
	}
	return bump(tx, k)
}

func (s *SQLite) Replace(
	ctx context.Context,
	k string,
	msgs []providers.Message,
	summary string,
	revision *int64,
) (bool, error) {
	applied := false
	err := transaction(ctx, s.db, func(tx *sql.Tx) error {
		if revision != nil {
			var cur int64
			e := tx.QueryRow("SELECT revision FROM sessions WHERE key=?", k).Scan(&cur)
			if e != nil && !errors.Is(e, sql.ErrNoRows) {
				return e
			}
			if cur != *revision {
				return nil
			}
		}
		if e := replaceSession(tx, k, msgs, &summary); e != nil {
			return e
		}
		applied = true
		return nil
	})
	return applied, err
}

func (s *SQLite) TruncateHistory(ctx context.Context, k string, n int) error {
	if n < 0 {
		n = 0
	}
	return transaction(ctx, s.db, func(tx *sql.Tx) error {
		if _, e := tx.Exec(
			"DELETE FROM session_messages WHERE key=? AND id NOT IN (SELECT id FROM session_messages WHERE key=? ORDER BY id DESC LIMIT ?)",
			k,
			k,
			n,
		); e != nil {
			return e
		}
		return bump(tx, k)
	})
}
func (s *SQLite) Compact(context.Context, string) error { return nil }
func (s *SQLite) ListSessions() []string {
	rows, e := s.db.Query("SELECT key FROM sessions ORDER BY key")
	if e != nil {
		return nil
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if e := rows.Scan(&k); e != nil {
			return nil
		}
		keys = append(keys, k)
	}
	if rows.Err() != nil {
		return nil
	}
	return keys
}

func (s *SQLite) ResolveSessionKey(ctx context.Context, k string) (string, bool, error) {
	var v string
	e := s.db.QueryRowContext(ctx, "SELECT key FROM sessions WHERE key=? UNION ALL SELECT key FROM session_aliases WHERE alias=? LIMIT 1", k, k).
		Scan(&v)
	if errors.Is(e, sql.ErrNoRows) {
		return "", false, nil
	}
	return v, e == nil, e
}

// SessionMeta holds the summary and routing scope of a SQLite session.
type SessionMeta struct {
	Key     string
	Summary string
	Scope   json.RawMessage
}

func (s *SQLite) GetSessionMeta(ctx context.Context, k string) (SessionMeta, error) {
	m := SessionMeta{Key: k}
	var scope []byte
	e := s.db.QueryRowContext(ctx, "SELECT summary,scope FROM sessions WHERE key=?", k).Scan(&m.Summary, &scope)
	m.Scope = scope
	if errors.Is(e, sql.ErrNoRows) {
		e = nil
	}
	return m, e
}

func (s *SQLite) UpsertSessionMeta(ctx context.Context, k string, scope json.RawMessage, aliases []string) error {
	return transaction(ctx, s.db, func(tx *sql.Tx) error {
		if e := ensureSession(tx, k); e != nil {
			return e
		}
		return saveSessionMetadata(tx, k, scope, aliases)
	})
}

func insertAliases(tx *sql.Tx, k string, aliases []string) error {
	for _, a := range aliases {
		if strings.TrimSpace(a) == "" || a == k || isMainSessionAlias(a) {
			continue
		}
		if _, e := tx.Exec(
			"INSERT INTO session_aliases(alias,key) VALUES(?,?) ON CONFLICT DO NOTHING",
			a,
			k,
		); e != nil {
			return e
		}
	}
	return nil
}

func isMainSessionAlias(alias string) bool {
	if alias == "" {
		return false
	}
	// Legacy form: "agent:main:main" (exactly 3 colon-separated parts)
	// Must not match "agent:sales:direct:main" etc.
	if strings.HasPrefix(alias, "agent:") && strings.HasSuffix(alias, ":main") {
		parts := strings.SplitN(alias, ":", 4)
		if len(parts) == 3 {
			return true
		}
	}
	// Opaque form: "sk_v1_" + SHA256("agent:main:main")
	if strings.HasPrefix(alias, "sk_v1_") {
		for _, agentID := range []string{"main", "Main", "MAIN"} {
			legacy := "agent:" + agentID + ":main"
			hash := sha256.Sum256([]byte(legacy))
			if "sk_v1_"+hex.EncodeToString(hash[:]) == alias {
				return true
			}
		}
	}
	return false
}
