package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	runtimeevents "github.com/sipeed/picoclaw/pkg/events"
	"github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/providers"
)

func TestSQLiteCompact_Overflow(t *testing.T) {
	cfg := testConfig(t)
	al := newCMTestAgentLoop(cfg)

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("expected default agent")
	}

	history := []providers.Message{
		{Role: "user", Content: "msg 1"},
		{Role: "assistant", Content: "resp 1"},
		{Role: "user", Content: "msg 2"},
		{Role: "assistant", Content: "resp 2"},
		{Role: "user", Content: "msg 3"},
	}
	defaultAgent.Sessions.SetHistory("session-overflow", history)

	runtimeCh, closeRuntimeEvents := subscribeRuntimeEventsForTest(
		t,
		al,
		16,
		runtimeevents.KindAgentContextCompress,
	)
	defer closeRuntimeEvents()

	err := al.contextManager.Compact(context.Background(), &CompactRequest{
		SessionKey: "session-overflow",
		Reason:     ContextCompressReasonRetry,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// After overflow compression, history should be shorter
	newHistory := defaultAgent.Sessions.GetHistory("session-overflow")
	if len(newHistory) >= len(history) {
		t.Fatalf("expected compressed history, got %d messages (was %d)", len(newHistory), len(history))
	}

	// Summary should contain compression note
	summary := defaultAgent.Sessions.GetSummary("session-overflow")
	if !strings.Contains(summary, "Emergency compression") {
		t.Fatalf("expected compression note in summary, got %q", summary)
	}

	// Event should carry the proactive reason
	events := collectRuntimeEventStream(runtimeCh)
	compressEvt, ok := findRuntimeEvent(events, runtimeevents.KindAgentContextCompress)
	if !ok {
		t.Fatal("expected context compress event")
	}
	payload, ok := compressEvt.Payload.(ContextCompressPayload)
	if !ok {
		t.Fatalf("expected ContextCompressPayload, got %T", compressEvt.Payload)
	}
	if payload.Reason != ContextCompressReasonRetry {
		t.Fatalf("expected retry reason, got %q", payload.Reason)
	}
}

func TestSQLiteCompact_Overflow_ProactiveReason(t *testing.T) {
	cfg := testConfig(t)
	al := newCMTestAgentLoop(cfg)

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("expected default agent")
	}

	history := []providers.Message{
		{Role: "user", Content: "msg 1"},
		{Role: "assistant", Content: "resp 1"},
		{Role: "user", Content: "msg 2"},
		{Role: "assistant", Content: "resp 2"},
		{Role: "user", Content: "msg 3"},
	}
	defaultAgent.Sessions.SetHistory("session-proactive", history)

	runtimeCh, closeRuntimeEvents := subscribeRuntimeEventsForTest(
		t,
		al,
		16,
		runtimeevents.KindAgentContextCompress,
	)
	defer closeRuntimeEvents()

	err := al.contextManager.Compact(context.Background(), &CompactRequest{
		SessionKey: "session-proactive",
		Reason:     ContextCompressReasonProactive,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := collectRuntimeEventStream(runtimeCh)
	compressEvt, ok := findRuntimeEvent(events, runtimeevents.KindAgentContextCompress)
	if !ok {
		t.Fatal("expected context compress event")
	}
	payload, ok := compressEvt.Payload.(ContextCompressPayload)
	if !ok {
		t.Fatalf("expected ContextCompressPayload, got %T", compressEvt.Payload)
	}
	if payload.Reason != ContextCompressReasonProactive {
		t.Fatalf("expected proactive reason, got %q", payload.Reason)
	}
}

func TestSQLiteCompact_Overflow_TooShortToCompress(t *testing.T) {
	cfg := testConfig(t)
	al := newCMTestAgentLoop(cfg)

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("expected default agent")
	}

	history := []providers.Message{
		{Role: "user", Content: "only one"},
	}
	defaultAgent.Sessions.SetHistory("session-tiny", history)

	err := al.contextManager.Compact(context.Background(), &CompactRequest{
		SessionKey: "session-tiny",
		Reason:     ContextCompressReasonRetry,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// History should be unchanged (too short to compress)
	newHistory := defaultAgent.Sessions.GetHistory("session-tiny")
	if len(newHistory) != len(history) {
		t.Fatalf("expected history unchanged, got %d messages (was %d)", len(newHistory), len(history))
	}
}

// ---------------------------------------------------------------------------
// Legacy Compact post-turn tests
// ---------------------------------------------------------------------------

func TestSQLiteCompact_PostTurn_BelowThreshold(t *testing.T) {
	cfg := testConfig(t)
	al := newCMTestAgentLoop(cfg)

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("expected default agent")
	}

	// Small history, below summarization thresholds
	history := []providers.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
	defaultAgent.Sessions.SetHistory("session-small", history)

	err := al.contextManager.Compact(context.Background(), &CompactRequest{
		SessionKey: "session-small",
		Reason:     ContextCompressReasonSummarize,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// History should remain unchanged
	newHistory := defaultAgent.Sessions.GetHistory("session-small")
	if len(newHistory) != len(history) {
		t.Fatalf("expected unchanged history, got %d messages (was %d)", len(newHistory), len(history))
	}
}

func TestSQLiteCompact_PostTurn_ExceedsMessageThreshold(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:                 t.TempDir(),
				ModelName:                 "test-model",
				MaxTokens:                 4096,
				MaxToolIterations:         10,
				ContextWindow:             8000,
				SummarizeMessageThreshold: 2,
				SummarizeTokenPercent:     75,
			},
		},
	}
	msgBus := bus.NewMessageBus()
	al := NewAgentLoop(cfg, msgBus, &simpleMockProvider{response: "summary"})

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("expected default agent")
	}

	// 6 messages > threshold of 2
	history := []providers.Message{
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2"},
		{Role: "user", Content: "q3"},
		{Role: "assistant", Content: "a3"},
	}
	defaultAgent.Sessions.SetHistory("session-threshold", history)

	runtimeCh, closeRuntimeEvents := subscribeRuntimeEventsForTest(
		t,
		al,
		16,
		runtimeevents.KindAgentSessionSummarize,
	)
	defer closeRuntimeEvents()

	err := al.contextManager.Compact(context.Background(), &CompactRequest{
		SessionKey: "session-threshold",
		Reason:     ContextCompressReasonSummarize,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	waitForRuntimeEvent(t, runtimeCh, 5*time.Second, func(evt runtimeevents.Event) bool {
		return evt.Kind == runtimeevents.KindAgentSessionSummarize
	})

	newHistory := defaultAgent.Sessions.GetHistory("session-threshold")
	if len(newHistory) >= len(history) {
		t.Fatalf("expected summarization to reduce history from %d messages, got %d", len(history), len(newHistory))
	}
}

// ---------------------------------------------------------------------------
// Legacy Ingest tests
// ---------------------------------------------------------------------------

type clearRecordingTurnStore struct {
	turnSessionStore
	cleared []memory.SessionWrite
}

func (s *clearRecordingTurnStore) ClearSession(ctx context.Context, write memory.SessionWrite) error {
	s.cleared = append(s.cleared, write)
	return s.turnSessionStore.ClearSession(ctx, write)
}

func TestClearCommandUsesRoutedAgentSessionStore(t *testing.T) {
	workspace := t.TempDir()
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         filepath.Join(workspace, "default"),
				ModelName:         "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
			List: []config.AgentConfig{
				{
					ID:        "main",
					Default:   true,
					Workspace: filepath.Join(workspace, "main"),
				},
				{
					ID:        "support",
					Workspace: filepath.Join(workspace, "support"),
				},
			},
			Dispatch: &config.DispatchConfig{
				Rules: []config.DispatchRule{
					{
						Name:  "support-dingtalk",
						Agent: "support",
						When: config.DispatchSelector{
							Channel: "dingtalk",
						},
					},
				},
			},
		},
		Session: config.SessionConfig{
			Dimensions: []string{"chat"},
		},
	}

	al := NewAgentLoop(cfg, bus.NewMessageBus(), &simpleMockProvider{response: "done"})
	t.Cleanup(al.Close)
	support, ok := al.registry.GetAgent("support")
	if !ok {
		t.Fatal("support agent is missing")
	}
	store := &clearRecordingTurnStore{turnSessionStore: support.turnStore}
	support.turnStore = store

	msg := testInboundMessage(bus.InboundMessage{
		Context: bus.InboundContext{
			Channel:  "dingtalk",
			ChatID:   "chat1",
			ChatType: "direct",
			SenderID: "user1",
		},
		Content: "/clear",
	})
	route, _, err := al.resolveMessageRoute(msg)
	if err != nil {
		t.Fatalf("resolveMessageRoute() error = %v", err)
	}
	sessionKey := al.allocateRouteSession(route, msg).SessionKey

	if _, err := al.processMessage(context.Background(), msg); err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}

	if len(store.cleared) != 1 || store.cleared[0].Key != sessionKey {
		t.Fatalf("clear requests = %+v, want one request for %q", store.cleared, sessionKey)
	}
}

// ---------------------------------------------------------------------------
// forceCompression edge cases (via legacy Compact)
// ---------------------------------------------------------------------------

func TestSQLiteCompact_Overflow_SingleTurnKeepsLastUserMessage(t *testing.T) {
	cfg := testConfig(t)
	al := newCMTestAgentLoop(cfg)

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("expected default agent")
	}

	// History with only 2 messages — forceCompression should still handle it
	history := []providers.Message{
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "first answer"},
	}
	defaultAgent.Sessions.SetHistory("session-2msg", history)

	err := al.contextManager.Compact(context.Background(), &CompactRequest{
		SessionKey: "session-2msg",
		Reason:     ContextCompressReasonRetry,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	newHistory := defaultAgent.Sessions.GetHistory("session-2msg")
	// With 2 messages, forceCompression returns false (len <= 2), so no compression
	if len(newHistory) != len(history) {
		t.Fatalf("expected no compression for 2-message history, got %d", len(newHistory))
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         t.TempDir(),
				ModelName:         "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}
}

func newCMTestAgentLoop(cfg *config.Config) *AgentLoop {
	msgBus := bus.NewMessageBus()
	return NewAgentLoop(cfg, msgBus, &simpleMockProvider{response: "test"})
}
