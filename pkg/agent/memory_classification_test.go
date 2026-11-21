package agent

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/providers"
)

type classificationProvider func(context.Context, []providers.Message, []providers.ToolDefinition) (*providers.LLMResponse, error)

func (classificationProvider) GetDefaultModel() string { return "fixture" }

func (p classificationProvider) Chat(
	ctx context.Context, msgs []providers.Message, defs []providers.ToolDefinition, _ string, _ map[string]any,
) (*providers.LLMResponse, error) {
	return p(ctx, msgs, defs)
}

func newClassificationAgent(t *testing.T, provider classificationProvider) (*AgentLoop, *AgentInstance) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = t.TempDir()
	cfg.Agents.Defaults.ModelName = "fixture"
	cfg.Memory.DisableAutoCapture = true // Explicit saves do not depend on scheduled review.
	loop := NewAgentLoop(cfg, bus.NewMessageBus(), provider)
	t.Cleanup(loop.Close)
	return loop, loop.GetRegistry().GetDefaultAgent()
}

const unclassifiedRecord = "我在曙光营地还有一枚奖励没领取"

func TestImmediateClassificationStartsAfterCommitAndUpdatesSameRecord(t *testing.T) {
	requests := make(chan []providers.Message, 1)
	release := make(chan struct{})
	var calls atomic.Int32
	loop, agent := newClassificationAgent(t, func(ctx context.Context, messages []providers.Message, defs []providers.ToolDefinition) (*providers.LLMResponse, error) {
		calls.Add(1)
		if len(defs) != 0 {
			return nil, errors.New("classification must not have tools")
		}
		requests <- messages
		select {
		case <-release:
			return &providers.LLMResponse{Content: `{"category":"日常/游戏","tags":["隐藏奖励","已有标签"],"aliases":["Dawn Camp"]}`}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	// A detached model call must survive the inbound context being canceled.
	inbound, cancelInbound := context.WithCancel(ctx)
	reply, err := loop.ProcessLocalCLI(inbound, `/memory add {"text":"`+unclassifiedRecord+`","date":"2026-05-21","tags":["已有标签"],"aliases":["曙光站"]}`, "cli:classify")
	cancelInbound()
	require.NoError(t, err)
	require.Equal(t, "已记录：2026-05-21（#1）", reply)
	select {
	case msgs := <-requests:
		require.Len(t, msgs, 2)
		require.Equal(t, memoryClassificationPrompt, msgs[0].Content)
		require.Contains(t, msgs[1].Content, unclassifiedRecord)
		require.NotContains(t, msgs[1].Content, "2026-05-21")
		require.NotContains(t, msgs[1].Content, "cli session")
	case <-ctx.Done():
		t.Fatal("no immediate classification request")
	}
	db := agent.memoryDB
	before, err := db.Search(ctx, memory.Search{Query: "曙光营地", Category: memory.OtherCategory})
	require.NoError(t, err)
	require.Len(t, before, 1)
	close(release)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		hits, searchErr := db.Search(ctx, memory.Search{Query: "Dawn Camp", Category: "日常/游戏", Tag: "隐藏奖励"})
		require.NoError(c, searchErr)
		require.Len(c, hits, 1)
		got := hits[0]
		require.Equal(c, before[0].ID, got.ID)
		require.Equal(c, before[0].Body, got.Body)
		require.Equal(c, before[0].Source, got.Source)
		require.Equal(c, before[0].Date, got.Date)
		require.Equal(c, before[0].CreatedAt, got.CreatedAt)
		require.NotEqual(c, before[0].UpdatedAt, got.UpdatedAt)
		require.ElementsMatch(c, []string{"已有标签", "隐藏奖励"}, got.Tags)
		require.ElementsMatch(c, []string{"曙光站", "Dawn Camp"}, got.Aliases)
	}, time.Second, 10*time.Millisecond)
	all, err := db.Search(ctx, memory.Search{Query: "曙光营地"})
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.EqualValues(t, 1, calls.Load())
	keys, err := db.DueReviews(ctx, time.Now(), time.Now())
	require.NoError(t, err)
	require.Empty(t, keys)
}

func TestImmediateClassificationSkipsKnownAndFailedInserts(t *testing.T) {
	var calls atomic.Int32
	loop, agent := newClassificationAgent(t, func(context.Context, []providers.Message, []providers.ToolDefinition) (*providers.LLMResponse, error) {
		calls.Add(1)
		return &providers.LLMResponse{Content: "null"}, nil
	})
	ctx := t.Context()
	for _, input := range []string{
		"帮我记一下，原神打到第三章",
		`/memory add {"text":"曙光营地","category":"日常/收集"}`,
		`/memory add {"text":"显式未分类","category":"other"}`,
	} {
		reply, err := loop.ProcessLocalCLI(ctx, input, "cli:classify")
		require.NoError(t, err)
		require.Contains(t, reply, "已记录")
	}
	reply, err := loop.ProcessLocalCLI(ctx, "帮我记一下，"+strings.Repeat("曙光营地", 501), "cli:classify")
	require.Error(t, err)
	require.Empty(t, reply)
	require.Never(t, func() bool { return calls.Load() != 0 }, 100*time.Millisecond, 5*time.Millisecond)
	got, ok, err := agent.memoryDB.RecordByID(ctx, 1)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "日常/游戏", got.Category)
}

func TestImmediateClassificationFromMemoryWrite(t *testing.T) {
	loop, agent := newClassificationAgent(t, func(_ context.Context, msgs []providers.Message, _ []providers.ToolDefinition) (*providers.LLMResponse, error) {
		if msgs[0].Content == memoryClassificationPrompt {
			return &providers.LLMResponse{Content: `{"category":"日常/收集","tags":["奖励"]}`}, nil
		}
		return memoryToolCall(memoryWriteName, map[string]any{"kind": "record", "action": "upsert", "text": unclassifiedRecord}), nil
	})
	ask := "保存一下，" + unclassifiedRecord
	reply := memoryWriteIM(t, loop, "weixin", "owner", "direct", ask, ask)
	require.Contains(t, reply, "（#1）")
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		hits, err := agent.memoryDB.Search(t.Context(), memory.Search{Query: "曙光营地", Category: "日常/收集"})
		require.NoError(c, err)
		require.Len(c, hits, 1)
		require.EqualValues(c, 1, hits[0].ID)
		require.Contains(c, hits[0].Source, "weixin message")
	}, time.Second, 10*time.Millisecond)
}

func TestImmediateClassificationLeavesOtherWhenModelDoesNotFinish(t *testing.T) {
	for _, fixture := range []struct {
		name, output string
		timeout      bool
		err          error
	}{
		{name: "uncertain", output: "null"},
		{name: "invalid", output: `{"category":"日常/收集","text":"try to overwrite"}`},
		{name: "unknown category", output: `{"category":"other","tags":["ignore"]}`},
		{name: "provider error", err: errors.New("offline")},
		{name: "timeout", timeout: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			_, agent := newClassificationAgent(t, func(ctx context.Context, _ []providers.Message, _ []providers.ToolDefinition) (*providers.LLMResponse, error) {
				if fixture.timeout {
					<-ctx.Done()
					// A late response must be discarded even if the provider
					// reports success instead of returning a timeout error.
					return &providers.LLMResponse{Content: `{"category":"日常/收集"}`}, nil
				}
				return &providers.LLMResponse{Content: fixture.output}, fixture.err
			})
			saved, classify, err := agent.memoryDB.AddImmediate(t.Context(), memory.Record{
				Body: unclassifiedRecord, Date: "2026-05-21", Source: "cli",
			})
			require.NoError(t, err)
			require.True(t, classify)
			ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
			defer cancel()
			agent.runImmediateClassification(ctx, saved)
			hits, err := agent.memoryDB.Search(t.Context(), memory.Search{Query: "曙光营地", Category: memory.OtherCategory})
			require.NoError(t, err)
			require.Equal(t, []memory.Record{saved}, hits)
		})
	}
}

func TestImmediateClassificationDoesNotDelayAgentClose(t *testing.T) {
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	defer close(release)
	loop, agent := newClassificationAgent(t, func(ctx context.Context, _ []providers.Message, _ []providers.ToolDefinition) (*providers.LLMResponse, error) {
		started <- ctx
		<-release // Simulate a slow provider that ignores cancellation.
		return &providers.LLMResponse{Content: `{"category":"日常/收集"}`}, nil
	})
	ctx := t.Context()
	reply, err := loop.ProcessLocalCLI(ctx, "帮我记一下，"+unclassifiedRecord, "cli:classify")
	require.NoError(t, err)
	require.Contains(t, reply, "已记录")
	var modelCtx context.Context
	select {
	case modelCtx = <-started:
	case <-time.After(time.Second):
		t.Fatal("classification did not start")
	}
	done := make(chan struct{})
	go func() {
		_ = agent.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("agent close waited for the classification goroutine")
	}
	require.ErrorIs(t, modelCtx.Err(), context.Canceled)
	reopened, err := memory.Open(memoryDatabasePath(agent.Workspace))
	require.NoError(t, err)
	defer reopened.Close()
	hits, err := reopened.Search(ctx, memory.Search{Query: "曙光营地", Category: memory.OtherCategory})
	require.NoError(t, err)
	require.Len(t, hits, 1)
}

func TestImmediateClassificationDiscardsStaleModelResult(t *testing.T) {
	for _, action := range []string{"edit", "delete"} {
		t.Run(action, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			defer close(release)
			_, agent := newClassificationAgent(t, func(ctx context.Context, _ []providers.Message, _ []providers.ToolDefinition) (*providers.LLMResponse, error) {
				close(started)
				select {
				case <-release:
				case <-ctx.Done():
				}
				return &providers.LLMResponse{Content: `{"category":"日常/收集","tags":["过期结果"]}`}, nil
			})
			ctx := t.Context()
			db := agent.memoryDB
			saved, _, err := db.AddImmediate(ctx, memory.Record{Body: unclassifiedRecord, Date: "2026-05-21", Source: "cli"})
			require.NoError(t, err)
			finished := make(chan struct{})
			go func() {
				agent.runImmediateClassification(ctx, saved)
				close(finished)
			}()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("classification did not start")
			}
			if action == "edit" {
				ok, err := db.Update(ctx, saved.ID, memory.Record{Body: "曙光营地的奖励已经领取"})
				require.NoError(t, err)
				require.True(t, ok)
			} else {
				ok, err := db.Delete(ctx, saved.ID)
				require.NoError(t, err)
				require.True(t, ok)
			}
			// Let the provider respond while its context is still valid.
			select {
			case release <- struct{}{}:
			case <-time.After(time.Second):
				t.Fatal("classification did not accept the result")
			}
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("classification did not finish")
			}
			r, ok, err := db.RecordByID(ctx, saved.ID)
			require.NoError(t, err)
			if action == "delete" {
				require.False(t, ok)
			} else {
				require.True(t, ok)
				require.Equal(t, "曙光营地的奖励已经领取", r.Body)
				require.Equal(t, memory.OtherCategory, r.Category)
				require.Empty(t, r.Tags)
			}
		})
	}
}
