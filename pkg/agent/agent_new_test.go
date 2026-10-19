package agent

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

func newConversationTestLoop(t *testing.T, provider providers.LLMProvider) (*AgentLoop, *bus.MessageBus) {
	t.Helper()
	cfg := &config.Config{
		Agents: config.AgentsConfig{Defaults: config.AgentDefaults{
			Workspace: t.TempDir(), ModelName: "test-model", MaxTokens: 4096,
			ContextWindow: 32768, MaxToolIterations: 5,
		}},
		Session: config.SessionConfig{Dimensions: []string{"chat"}},
	}
	msgBus := bus.NewMessageBus()
	al := NewAgentLoop(cfg, msgBus, provider)
	t.Cleanup(al.Close)
	return al, msgBus
}

func newConversationTestMessage(content, chatID string) bus.InboundMessage {
	return testInboundMessage(bus.InboundMessage{
		Context: bus.InboundContext{Channel: "telegram", ChatID: chatID, ChatType: "direct", SenderID: "user1"},
		Content: content,
	})
}

func TestProcessMessage_NewConversation(t *testing.T) {
	var calls [][]providers.Message
	provider := &capturingMockProvider{response: "新回答", captureFn: func(messages []providers.Message) {
		calls = append(calls, append([]providers.Message(nil), messages...))
	}}
	al, _ := newConversationTestLoop(t, provider)
	agent := al.registry.GetDefaultAgent()
	msg := newConversationTestMessage("/new", "chat1")
	route, _, err := al.resolveMessageRoute(msg)
	if err != nil {
		t.Fatal(err)
	}
	key := al.allocateRouteSession(route, msg).SessionKey
	agent.Sessions.AddMessage(key, "user", "旧对话暗号")
	agent.Sessions.SetSummary(key, "旧摘要暗号")

	reply, err := al.processMessage(t.Context(), msg)
	if err != nil || reply != "Chat history cleared!" || len(calls) != 0 {
		t.Fatalf("/new reply = %q, error = %v, calls = %d", reply, err, len(calls))
	}
	if len(agent.Sessions.GetHistory(key)) != 0 || agent.Sessions.GetSummary(key) != "" {
		t.Fatal("/new did not clear history and summary")
	}

	// Re-seed old state to verify that /new with a prompt also performs the reset.
	agent.Sessions.AddMessage(key, "user", "旧对话暗号")
	agent.Sessions.SetSummary(key, "旧摘要暗号")
	for i, prompt := range []string{"帮我查查今天成都的天气", "看看今天有啥新咖啡"} {
		msg.Content = "/new " + prompt
		reply, err = al.processMessage(t.Context(), msg)
		if err != nil || reply != "新回答" || len(calls) != i+1 {
			t.Fatalf("reply = %q, error = %v, calls = %d", reply, err, len(calls))
		}
		messages := calls[i]
		var userMessages []string
		for _, message := range messages {
			if strings.Contains(message.Content, "旧对话暗号") || strings.Contains(message.Content, "旧摘要暗号") {
				t.Fatalf("old context leaked: %+v", messages)
			}
			if message.Role == "user" {
				userMessages = append(userMessages, message.Content)
			}
		}
		if !reflect.DeepEqual(userMessages, []string{prompt}) {
			t.Fatalf("user messages = %q, want only %q", userMessages, prompt)
		}
		history := agent.Sessions.GetHistory(key)
		if len(history) != 2 || history[0].Content != prompt || history[1].Content != "新回答" {
			t.Fatalf("new history = %+v", history)
		}
	}

	msg.Content = "展开说说"
	if _, err := al.processMessage(t.Context(), msg); err != nil {
		t.Fatal(err)
	}
	var userMessages []string
	for _, message := range calls[2] {
		if message.Role == "user" {
			userMessages = append(userMessages, message.Content)
		}
	}
	if !reflect.DeepEqual(userMessages, []string{"看看今天有啥新咖啡", "展开说说"}) {
		t.Fatalf("follow-up context = %q", userMessages)
	}
	reopened := initSessionStore(filepath.Join(agent.Workspace, "sessions"))
	t.Cleanup(func() { _ = reopened.Close() })
	if history := reopened.GetHistory(key); len(history) != 4 || history[0].Content != "看看今天有啥新咖啡" {
		t.Fatalf("persisted history = %+v", history)
	}
}

type newConversationClearFailure struct{ ContextManager }

func (newConversationClearFailure) Clear(context.Context, string) error {
	return errors.New("storage unavailable")
}

func TestProcessMessage_NewConversationClearFailure(t *testing.T) {
	provider := &countingMockProvider{response: "must not run"}
	al, _ := newConversationTestLoop(t, provider)
	al.contextManager = newConversationClearFailure{al.contextManager}
	reply, err := al.processMessage(t.Context(), newConversationTestMessage("/new question", "chat1"))
	if err != nil || !strings.Contains(reply, "storage unavailable") || provider.calls != 0 {
		t.Fatalf("reply = %q, error = %v, calls = %d", reply, err, provider.calls)
	}
}

func TestProcessMessage_NewConversationPromptIsLiteral(t *testing.T) {
	provider := &recordingProvider{}
	al, _ := newConversationTestLoop(t, provider)
	for _, prompt := range []string{"第一行  内容\n第二行\t内容", "/clear"} {
		_, err := al.processMessage(t.Context(), newConversationTestMessage("/new@bot\t"+prompt, "chat1"))
		if err != nil {
			t.Fatal(err)
		}
		messages := provider.lastMessages
		if len(messages) == 0 || messages[len(messages)-1].Content != prompt {
			t.Fatalf("prompt %q not preserved: %+v", prompt, messages)
		}
	}
}

func TestProcessMessage_NewConversationDiscardsPendingSummary(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	provider := &blockingDirectProvider{
		firstStarted: started, releaseFirst: release, firstResp: "旧摘要暗号", finalResp: "新回答",
	}
	al, _ := newConversationTestLoop(t, provider)
	agent := al.registry.GetDefaultAgent()
	msg := newConversationTestMessage("/new 新问题", "chat1")
	route, _, err := al.resolveMessageRoute(msg)
	if err != nil {
		t.Fatal(err)
	}
	key := al.allocateRouteSession(route, msg).SessionKey
	for range 6 {
		agent.Sessions.AddMessage(key, "user", "旧问题")
		agent.Sessions.AddMessage(key, "assistant", "旧回答")
	}
	manager := al.contextManager.(*legacyContextManager)
	done := make(chan struct{})
	go func() { defer close(done); manager.summarizeSession(agent, key) }()
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("summary did not finish")
		}
	})
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("summary did not start")
	}
	if reply, err := al.processMessage(t.Context(), msg); err != nil || reply != "新回答" {
		t.Fatalf("new reply = %q, error = %v", reply, err)
	}
	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("summary did not finish")
	}
	if summary := agent.Sessions.GetSummary(key); summary != "" {
		t.Fatalf("old summary restored: %q", summary)
	}
	if history := agent.Sessions.GetHistory(key); len(history) != 2 || history[0].Content != "新问题" {
		t.Fatalf("old summary altered new history: %+v", history)
	}
}

func startNewConversationTestLoop(t *testing.T, al *AgentLoop) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- al.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run() error = %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Run() did not stop")
		}
	})
}

func publishNewConversationTestMessage(t *testing.T, msgBus *bus.MessageBus, msg bus.InboundMessage) {
	t.Helper()
	if err := msgBus.PublishInbound(t.Context(), msg); err != nil {
		t.Fatal(err)
	}
}

func receiveNewConversationTestReply(t *testing.T, msgBus *bus.MessageBus) bus.OutboundMessage {
	t.Helper()
	select {
	case msg := <-msgBus.OutboundChan():
		return msg
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for reply")
		return bus.OutboundMessage{}
	}
}

func waitNewConversationTestCondition(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !ready() {
		select {
		case <-deadline.C:
			t.Fatal("timed out waiting for session state")
		case <-ticker.C:
		}
	}
}

func TestAgentLoop_NewConversationRejectsBusySession(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	provider := &blockingDirectProvider{
		firstStarted: started, releaseFirst: release, firstResp: "原任务完成", finalResp: "其他任务完成",
	}
	al, msgBus := newConversationTestLoop(t, provider)
	al.workerSem = make(chan struct{}, 2)
	startNewConversationTestLoop(t, al)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	msg := newConversationTestMessage("帮我查天气", "chat1")
	route, _, err := al.resolveMessageRoute(msg)
	if err != nil {
		t.Fatal(err)
	}
	key := al.allocateRouteSession(route, msg).SessionKey
	publishNewConversationTestMessage(t, msgBus, msg)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("original request did not start")
	}
	agent := al.registry.GetDefaultAgent()
	before := agent.Sessions.GetHistory(key)
	for _, command := range []string{"/new", "/new 新问题"} {
		msg.Content = command
		publishNewConversationTestMessage(t, msgBus, msg)
		reply := receiveNewConversationTestReply(t, msgBus)
		if reply.Content != "当前有个「帮我查天气」任务在执行，请待会再试。" ||
			reply.Context.Raw[metadataKeyOutboundKind] != outboundKindCommandReply {
			t.Fatalf("busy reply = %+v", reply)
		}
		if !reflect.DeepEqual(before, agent.Sessions.GetHistory(key)) || al.GetActiveTurnBySession(key) == nil {
			t.Fatal("/new altered the running task or its history")
		}
		if al.pendingSteeringCountForScope(key) != 0 {
			t.Fatal("/new was queued as steering")
		}
	}
	// A different chat can reset and run its own prompt while chat1 is busy.
	publishNewConversationTestMessage(t, msgBus, newConversationTestMessage("/new 其他问题", "chat2"))
	if reply := receiveNewConversationTestReply(t, msgBus); reply.Content != "其他任务完成" || reply.ChatID != "chat2" {
		t.Fatalf("other chat reply = %+v", reply)
	}
	close(release)
	if reply := receiveNewConversationTestReply(t, msgBus); reply.Content != "原任务完成" {
		t.Fatalf("original reply = %+v", reply)
	}
	waitNewConversationTestCondition(t, func() bool { return al.GetActiveTurnBySession(key) == nil })
	provider.mu.Lock()
	calls := provider.calls
	provider.mu.Unlock()
	if calls != 2 {
		t.Fatalf("calls = %d, rejected prompt must not execute", calls)
	}
}

func TestAgentLoop_NewConversationRejectsPendingSession(t *testing.T) {
	provider := &countingMockProvider{response: "原任务完成"}
	al, msgBus := newConversationTestLoop(t, provider)
	// Reserve the only worker slot so the first message stays in setup.
	al.workerSem <- struct{}{}
	startNewConversationTestLoop(t, al)
	msg := newConversationTestMessage("等待执行的问题", "chat1")
	route, _, err := al.resolveMessageRoute(msg)
	if err != nil {
		t.Fatal(err)
	}
	key := al.allocateRouteSession(route, msg).SessionKey
	publishNewConversationTestMessage(t, msgBus, msg)
	waitNewConversationTestCondition(t, func() bool { return al.GetActiveTurnBySession(key) != nil })
	msg.Content = "/new 不应该执行"
	publishNewConversationTestMessage(t, msgBus, msg)
	if reply := receiveNewConversationTestReply(t, msgBus); reply.Content != "当前有个「等待执行的问题」任务在执行，请待会再试。" {
		t.Fatalf("pending reply = %+v", reply)
	}
	if al.pendingSteeringCountForScope(key) != 0 {
		t.Fatal("/new was queued")
	}
	<-al.workerSem
	if reply := receiveNewConversationTestReply(t, msgBus); reply.Content != "原任务完成" {
		t.Fatalf("original reply = %+v", reply)
	}
}

func TestAgentLoop_NewConversationRejectsRunningTool(t *testing.T) {
	started := make(chan struct{})
	provider := &toolCallProvider{
		toolCalls: []providers.ToolCall{
			{ID: "call_1", Type: "function", Name: "busy_tool", Arguments: map[string]any{}},
		},
	}
	al, msgBus := newConversationTestLoop(t, provider)
	al.RegisterTool(&interruptibleTool{name: "busy_tool", started: started})
	startNewConversationTestLoop(t, al)
	msg := newConversationTestMessage("工具任务", "chat1")
	publishNewConversationTestMessage(t, msgBus, msg)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("tool did not start")
	}
	msg.Content = "/new 新问题"
	publishNewConversationTestMessage(t, msgBus, msg)
	for {
		reply := receiveNewConversationTestReply(t, msgBus)
		if reply.Context.Raw[metadataKeyOutboundKind] == outboundKindCommandReply {
			if reply.Content != "当前有个「工具任务」任务在执行，请待会再试。" {
				t.Fatalf("busy reply = %+v", reply)
			}
			break
		}
	}
	provider.mu.Lock()
	calls := provider.calls
	provider.mu.Unlock()
	if calls != 1 {
		t.Fatalf("calls = %d, running tool must not be canceled", calls)
	}
}

func TestRejectNewCommand_UnicodeTaskName(t *testing.T) {
	al, msgBus := newConversationTestLoop(t, &simpleMockProvider{})
	active := &turnState{userMessage: strings.Repeat("成", 100) + "\n  天气"}
	msg := newConversationTestMessage("/NEW@bot question", "chat1")
	msg.Context.Account, msg.Context.TopicID, msg.MessageID = "bot1", "42", "incoming-1"
	msg.Context.ReplyHandles = map[string]string{"reply_token": "handle"}
	if err := al.enqueueSteeringMessage("key", "main", providers.Message{
		Role: "user", Content: "existing follow-up",
	}); err != nil {
		t.Fatal(err)
	}
	if !al.tryRejectNewCommand(t.Context(), msg, "key", active) {
		t.Fatal("/NEW@bot not recognized")
	}
	reply := receiveNewConversationTestReply(t, msgBus)
	if !utf8.ValidString(reply.Content) || !strings.Contains(reply.Content, strings.Repeat("成", 77)+"...") {
		t.Fatalf("invalid truncated task name: %q", reply.Content)
	}
	if reply.Context.Account != "bot1" || reply.Context.TopicID != "42" ||
		reply.ReplyToMessageID != "incoming-1" || reply.Context.ReplyHandles["reply_token"] != "handle" {
		t.Fatalf("reply lost addressing context: %+v", reply.Context)
	}
	if al.pendingSteeringCountForScope("key") != 1 {
		t.Fatal("busy rejection changed the existing steering queue")
	}
}
