package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Lease holds a shared lock for a running agent or an exclusive maintenance lock.
// The OS releases the lock even if the process crashes.
func Lease(path string, exclusive bool) (func(), error) {
	dir := filepath.Dir(path)
	if e := os.MkdirAll(dir, 0o700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(dir, "memory.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if e != nil {
		return nil, e
	}
	if e = lockFile(f, exclusive); e != nil {
		f.Close()
		return nil, fmt.Errorf("memory database in use; stop Gateway and CLI agents first: %w", e)
	}
	return func() { _ = f.Close() }, nil
}

func checkSnapshot(ctx context.Context, path string) error {
	db, e := sql.Open("sqlite", sqliteURI(path, "mode=ro&_pragma=query_only(1)"))
	if e != nil {
		return e
	}
	defer db.Close()
	var version int
	if e = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); e != nil {
		return e
	}
	if e = checkSchema(ctx, db, version); e != nil {
		return e
	}
	var result string
	if e = db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); e != nil {
		return e
	}
	if result != "ok" {
		return errors.New("memory snapshot integrity check failed")
	}
	tables := []string{
		"sessions",
		"session_aliases",
		"session_messages",
		"preferences",
		"records",
		"records_fts",
		"memory_meta",
		"memory_review_notes",
	}
	for _, table := range tables {
		if _, e = db.ExecContext(ctx, "SELECT 1 FROM "+table+" LIMIT 0"); e != nil {
			return e
		}
	}
	return nil
}

func Backup(ctx context.Context, path, dest string) error {
	abs, e := filepath.Abs(path)
	if e != nil {
		return e
	}
	dest, e = filepath.Abs(dest)
	if e != nil {
		return e
	}
	if abs == dest {
		return errors.New("backup destination must differ from database")
	}
	if _, e = os.Stat(abs); e != nil {
		return e
	}
	out, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if e != nil {
		return e
	}
	if e = out.Close(); e != nil {
		os.Remove(dest)
		return e
	}
	ok := false
	defer func() {
		if !ok {
			os.Remove(dest)
		}
	}()
	db, e := sql.Open("sqlite", sqliteURI(abs, "mode=ro&_pragma=busy_timeout(5000)"))
	if e != nil {
		return e
	}
	defer db.Close()
	if _, e = db.ExecContext(ctx, "VACUUM INTO ?", dest); e != nil {
		return e
	}
	if e = checkSnapshot(ctx, dest); e != nil {
		return e
	}
	f, e := os.OpenFile(dest, os.O_RDWR, 0o600)
	if e != nil {
		return e
	}
	if e = errors.Join(f.Sync(), f.Close()); e != nil {
		return e
	}
	ok = true
	return nil
}

func Restore(ctx context.Context, path, src string) (string, error) {
	if e := checkSnapshot(ctx, src); e != nil {
		return "", e
	}
	for _, p := range []string{src, path} {
		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			if _, e := os.Stat(p + suffix); e == nil {
				return "", fmt.Errorf("database side file exists: %s; use a clean SQLite snapshot", p+suffix)
			} else if !errors.Is(e, os.ErrNotExist) {
				return "", e
			}
		}
	}
	dir := filepath.Dir(path)
	if e := os.MkdirAll(dir, 0o700); e != nil {
		return "", e
	}
	f, e := os.CreateTemp(dir, ".restore-*.sqlite3")
	if e != nil {
		return "", e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	in, e := os.Open(src)
	if e != nil {
		return "", e
	}
	defer in.Close()
	if _, e = io.Copy(f, in); e != nil {
		return "", e
	}
	if e = errors.Join(f.Sync(), f.Close()); e != nil {
		return "", e
	}
	if e = checkSnapshot(ctx, f.Name()); e != nil {
		return "", e
	}
	var before string
	if _, e = os.Stat(path); e == nil {
		before = path + ".before-" + time.Now().UTC().Format("20060102T150405.000000000Z")
		if e = Backup(ctx, path, before); e != nil {
			return "", e
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return "", e
	}
	if e = os.Rename(f.Name(), path); e != nil {
		return before, e
	}
	return before, nil
}
