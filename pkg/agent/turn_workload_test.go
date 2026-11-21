package agent

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
)

type turnIOProvider struct {
	batches, fanout int
	calls           int
	beforeCall      func(int)
}

func (p *turnIOProvider) GetDefaultModel() string { return "test-model" }
func (p *turnIOProvider) Chat(
	_ context.Context, messages []providers.Message, _ []providers.ToolDefinition, _ string, _ map[string]any,
) (*providers.LLMResponse, error) {
	p.calls++
	if p.beforeCall != nil {
		p.beforeCall(p.calls)
	}
	if messages[len(messages)-1].Role != "tool" {
		p.calls = 1
	}
	if p.calls > p.batches {
		return &providers.LLMResponse{Content: "done"}, nil
	}
	calls := make([]providers.ToolCall, 0, p.fanout)
	for i := range p.fanout {
		calls = append(
			calls,
			providers.ToolCall{
				ID:        fmt.Sprintf("call-%d-%d", p.calls, i),
				Type:      "function",
				Name:      "turn_io_fixture",
				Arguments: map[string]any{},
			},
		)
	}
	return &providers.LLMResponse{ToolCalls: calls}, nil
}

type turnIOTool struct {
	result     string
	seen       int
	beforeCall func(int)
}

func (*turnIOTool) Name() string               { return "turn_io_fixture" }
func (*turnIOTool) Description() string        { return "Deterministic tool for SQLite transaction tests" }
func (*turnIOTool) Parameters() map[string]any { return map[string]any{"type": "object"} }
func (t *turnIOTool) Execute(_ context.Context, _ map[string]any) *tools.ToolResult {
	t.seen++
	if t.beforeCall != nil {
		t.beforeCall(t.seen)
	}
	return tools.SilentResult(t.result)
}

func setupTurnIO(t *testing.T, p *turnIOProvider, tool *turnIOTool) (*AgentLoop, *AgentInstance) {
	t.Helper()
	al, _ := newConversationTestLoop(t, p)
	a := al.GetRegistry().GetDefaultAgent()
	a.MaxIterations = 70
	a.ContextWindow = 1 << 25
	a.SummarizeMessageThreshold = 1 << 20
	a.SummarizeTokenPercent = 95
	al.RegisterTool(tool)
	return al, a
}

// This workload also compiles against 6441d1f1. When called explicitly with
// PICOCLAW_SQLITE_WORKLOAD, run repeated IM turns and report the database size
// and revision delta for comparison under a file syscall tracer.
func TestSQLiteTurnWorkload(t *testing.T) {
	mode := os.Getenv("PICOCLAW_SQLITE_WORKLOAD")
	if mode == "" {
		t.Skip("set PICOCLAW_SQLITE_WORKLOAD to ordinary, tools or long")
	}
	var batches int
	switch mode {
	case "ordinary":
		batches = 0
	case "tools":
		batches = 3
	case "long":
		batches = 18
	default:
		t.Fatal("unknown SQLite workload")
	}
	al, a := setupTurnIO(
		t,
		&turnIOProvider{batches: batches, fanout: 2},
		&turnIOTool{result: strings.Repeat("fixture result ", 15)},
	)
	msg := newConversationTestMessage("measure writes", "io-workload")
	route, _, err := al.resolveMessageRoute(msg)
	require.NoError(t, err)
	key := al.allocateRouteSession(route, msg).SessionKey
	for i := range 12 {
		_, err = al.processMessage(t.Context(), msg)
		require.NoError(t, err)
		if i == 0 {
			info, statErr := os.Stat(memoryDatabasePath(a.Workspace))
			require.NoError(t, statErr)
			t.Logf("first-turn database bytes: %d", info.Size())
		}
	}
	h, _, rev, err := a.memoryDB.Snapshot(t.Context(), key)
	require.NoError(t, err)
	info, err := os.Stat(memoryDatabasePath(a.Workspace))
	require.NoError(t, err)
	db, err := sql.Open("sqlite", "file:"+memoryDatabasePath(a.Workspace)+"?mode=ro")
	require.NoError(t, err)
	defer db.Close()
	var free, pages, synchronous int
	var journal string
	require.NoError(t, db.QueryRow("PRAGMA freelist_count").Scan(&free))
	require.NoError(t, db.QueryRow("PRAGMA page_count").Scan(&pages))
	require.NoError(t, db.QueryRow("PRAGMA journal_mode").Scan(&journal))
	require.NoError(t, db.QueryRow("PRAGMA synchronous").Scan(&synchronous))
	t.Logf("mode=%s turns=12 messages=%d revision=%d db_bytes=%d free_pages=%d pages=%d journal=%s synchronous=%d",
		mode, len(h), rev, info.Size(), free, pages, journal, synchronous)
}
