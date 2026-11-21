package agent

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/providers"
)

func TestSQLiteTurnsCommitWholeBatches(t *testing.T) {
	for _, case_ := range []struct {
		name                string
		batches, rev, bytes int
	}{
		{"ordinary", 0, 1, 1}, {"tools", 3, 1, 100}, {"by batch count", 18, 3, 100}, {"by byte count", 2, 3, 300 << 10},
	} {
		t.Run(case_.name, func(t *testing.T) {
			al, a := setupTurnIO(
				t,
				&turnIOProvider{batches: case_.batches, fanout: 2},
				&turnIOTool{result: strings.Repeat("fixture. ", case_.bytes/9+1)},
			)
			msg := newConversationTestMessage("measure this turn", "whole-batches")
			msg.PrivateChat = &bus.PrivateChat{Text: msg.Content}
			_, err := al.processMessage(t.Context(), msg)
			require.NoError(t, err)
			route, _, err := al.resolveMessageRoute(msg)
			require.NoError(t, err)
			key := al.allocateRouteSession(route, msg).SessionKey
			snap, err := a.memoryDB.ReadSession(t.Context(), key)
			require.NoError(t, err)
			require.Len(t, snap.Messages, 2+case_.batches*3)
			require.True(t, completeToolBatches(snap.Messages))
			require.EqualValues(t, case_.rev, snap.Revision)
			require.Error(t, a.memoryDB.CheckSessionAccess(t.Context(), key, false))
			meta, err := a.memoryDB.GetSessionMeta(t.Context(), key)
			require.NoError(t, err)
			require.NotEmpty(t, meta.Scope)
		})
	}
}

func TestSQLiteTurnCrash(t *testing.T) {
	mode := os.Getenv("PICOCLAW_MEMORY_CRASH_MODE")
	if mode != "" {
		crashTurnChild(t, mode)
		return
	}
	for _, mode := range []string{"before commit", "after checkpoint", "incomplete batch"} {
		t.Run(mode, func(t *testing.T) {
			path := memory.Path(t.TempDir())
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSQLiteTurnCrash$")
			cmd.Env = append(os.Environ(), "PICOCLAW_MEMORY_CRASH_MODE="+mode, "PICOCLAW_MEMORY_CRASH_DB="+path)
			out, err := cmd.StdoutPipe()
			require.NoError(t, err)
			cmd.Stderr = cmd.Stdout
			require.NoError(t, cmd.Start())
			defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
			sc := bufio.NewScanner(out)
			var confirmed, paused bool
			var key string
			var log strings.Builder
			for sc.Scan() {
				line := sc.Text()
				log.WriteString(line + "\n")
				if line == "PRIVATE SAVES CONFIRMED" {
					confirmed = true
				}
				if k, ok := strings.CutPrefix(line, "PAUSE "); ok {
					key = k
					paused = true
					break
				}
			}
			require.True(t, paused, log.String())
			require.True(t, confirmed, log.String())
			require.NoError(t, cmd.Process.Kill())
			require.Error(t, cmd.Wait())
			db, err := memory.Open(path)
			require.NoError(t, err)
			defer db.Close()
			snap, err := db.ReadSession(t.Context(), key)
			require.NoError(t, err)
			if mode == "before commit" {
				require.Empty(t, snap.Messages)
			} else {
				require.Len(t, snap.Messages, 1+8*3)
				require.EqualValues(t, 1, snap.Revision)
				require.True(t, completeToolBatches(snap.Messages))
			}
			pref, err := db.Preferences(t.Context())
			require.NoError(t, err)
			require.Equal(t, []memory.Preference{{Key: "style", Value: "中文简洁"}}, pref)
			hits, err := db.Search(t.Context(), memory.Search{Query: "原神"})
			require.NoError(t, err)
			require.Len(t, hits, 1)
		})
	}
}

func crashTurnChild(t *testing.T, mode string) {
	savedPath := memoryDatabasePath
	memoryDatabasePath = func(string) string { return os.Getenv("PICOCLAW_MEMORY_CRASH_DB") }
	defer func() { memoryDatabasePath = savedPath }()

	p := &turnIOProvider{batches: 12, fanout: 2}
	tool := &turnIOTool{result: "a saved tool result"}
	al, _ := setupTurnIO(t, p, tool)
	msg := newConversationTestMessage("", "crash-fixture")
	msg.Channel, msg.Context.Channel = "weixin", "weixin"
	msg.PrivateChat = &bus.PrivateChat{}
	send := func(text string) string {
		msg.Content, msg.PrivateChat.Text = text, text
		response, err := al.processMessage(t.Context(), msg)
		require.NoError(t, err)
		return response
	}
	require.Contains(t, send("/preference set style 中文简洁"), "已保存")
	require.Contains(t, send("/memory add 原神玩到第三章"), "已记录")
	fmt.Println("PRIVATE SAVES CONFIRMED")
	route, _, err := al.resolveMessageRoute(msg)
	require.NoError(t, err)
	key := al.allocateRouteSession(route, msg).SessionKey
	pause := func() {
		fmt.Println("PAUSE " + key)
		// The parent sends SIGKILL, so no deferred shutdown or flushing can run.
		for {
			time.Sleep(time.Hour)
		}
	}
	p.beforeCall = func(n int) {
		if (mode == "before commit" && n == 1) || (mode == "after checkpoint" && n == 9) {
			pause()
		}
	}
	tool.beforeCall = func(n int) {
		if mode == "incomplete batch" && n == 18 {
			pause()
		}
	}
	_ = send("begin crash fixture")
	t.Fatal("did not reach pause point")
}

func TestHardAbortRemovesOnlyCommittedBatchesOfCurrentTurn(t *testing.T) {
	p := &turnIOProvider{fanout: 2}
	al, a := setupTurnIO(t, p, &turnIOTool{result: "fixture"})
	msg := newConversationTestMessage("initial successful turn", "abort-fixture")
	_, err := al.processMessage(t.Context(), msg)
	require.NoError(t, err)
	route, _, err := al.resolveMessageRoute(msg)
	require.NoError(t, err)
	key := al.allocateRouteSession(route, msg).SessionKey
	old, err := a.memoryDB.ReadSession(t.Context(), key)
	require.NoError(t, err)
	p.batches = 12
	p.calls = 0
	p.beforeCall = func(n int) {
		if n == 9 {
			snap, e := a.memoryDB.ReadSession(t.Context(), key)
			require.NoError(t, e)
			require.Len(t, snap.Messages, len(old.Messages)+1+8*3)
			require.NoError(t, al.HardAbort(key))
		}
	}
	msg.Content = "abort this turn after the checkpoint"
	_, err = al.processMessage(t.Context(), msg)
	require.NoError(t, err)
	after, err := a.memoryDB.ReadSession(t.Context(), key)
	require.NoError(t, err)
	require.Equal(t, old.RowIDs, after.RowIDs)
	require.Equal(t, old.Messages, after.Messages)
}

type turnMemoryRetryProvider struct {
	t     *testing.T
	calls int
}

func (*turnMemoryRetryProvider) GetDefaultModel() string { return "test-model" }
func (p *turnMemoryRetryProvider) Chat(
	_ context.Context, msgs []providers.Message, _ []providers.ToolDefinition, _ string, _ map[string]any,
) (*providers.LLMResponse, error) {
	p.calls++
	if p.calls == 1 {
		return &providers.LLMResponse{ToolCalls: []providers.ToolCall{{
			ID: "one", Type: "function", Name: "turn_io_fixture", Arguments: map[string]any{},
		}}}, nil
	}
	var calls, results int
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			if tc.ID == "one" {
				calls++
			}
		}
		if m.Role == "tool" && m.ToolCallID == "one" {
			results++
			require.Contains(p.t, m.Content, "fixture response")
		}
	}
	require.Equal(p.t, 1, calls)
	require.Equal(p.t, 1, results)
	if p.calls == 2 {
		return nil, fmt.Errorf("InvalidParameter: Total tokens of image and text exceed max message tokens")
	}
	return &providers.LLMResponse{Content: "done after retry"}, nil
}

func TestContextRetryKeepsLiveToolBatchWithoutRewritingHistory(t *testing.T) {
	p := &turnMemoryRetryProvider{t: t}
	al, _ := newConversationTestLoop(t, p)
	al.RegisterTool(&turnIOTool{result: "fixture response"})
	agent := al.GetRegistry().GetDefaultAgent()
	agent.SummarizeMessageThreshold = 100
	agent.SummarizeTokenPercent = 95
	msg := newConversationTestMessage("use the tool", "retry-fixture")
	route, _, err := al.resolveMessageRoute(msg)
	require.NoError(t, err)
	key := al.allocateRouteSession(route, msg).SessionKey
	h := []providers.Message{
		{Role: "user", Content: "old"},
		{Role: "assistant", Content: "first"},
		{Role: "user", Content: "old again"},
		{Role: "assistant", Content: "second"},
	}
	agent.Sessions.SetHistory(key, h)
	old, err := agent.memoryDB.ReadSession(t.Context(), key)
	require.NoError(t, err)
	response, err := al.processMessage(t.Context(), msg)
	require.NoError(t, err)
	require.Equal(t, "done after retry", response)
	require.Equal(t, 3, p.calls)
	after, err := agent.memoryDB.ReadSession(t.Context(), key)
	require.NoError(t, err)
	require.Equal(t, old.RowIDs, after.RowIDs[:len(old.RowIDs)])
	require.Equal(t, old.Revision+1, after.Revision)
	require.Len(t, after.Messages, len(h)+4)
}
