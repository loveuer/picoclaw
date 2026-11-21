package agent

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	// Tests can construct independent agents in one process with reused keys.
	dir, err := os.MkdirTemp("", "picoclaw-agent-db-tests-")
	if err != nil {
		panic(err)
	}
	memoryDatabasePath = func(workspace string) string {
		return filepath.Join(dir, fmt.Sprintf("%x", sha256.Sum256([]byte(workspace))), "memory.sqlite3")
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
