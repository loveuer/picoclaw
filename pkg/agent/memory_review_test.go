package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/providers"
)

type memoryReviewTestChannels struct{ recordingChannelManager }

func (*memoryReviewTestChannels) GetChannel(name string) (channels.Channel, bool) {
	return &fakeChannel{}, name == "weixin"
}

type reviewTestProvider struct {
	t       *testing.T
	reviews int
}

func (*reviewTestProvider) GetDefaultModel() string { return "review-test" }

func (p *reviewTestProvider) Chat(
	_ context.Context, msgs []providers.Message, defs []providers.ToolDefinition, _ string, _ map[string]any,
) (*providers.LLMResponse, error) {
	if len(msgs) != 2 || msgs[0].Content != memoryReviewPrompt {
		for _, def := range defs {
			require.NotEqual(p.t, "memory_capture", def.Function.Name)
		}
		return &providers.LLMResponse{Content: "收到"}, nil
	}
	require.Empty(p.t, defs)
	p.reviews++
	var in reviewModelInput
	require.NoError(p.t, json.Unmarshal([]byte(msgs[1].Content), &in))
	require.Len(p.t, in.Notes, 2)
	require.Equal(p.t, "我平时喜欢用中文简短回答", in.Notes[0].Text)
	require.Equal(p.t, "我在2026-05-21把原神打到第三章", in.Notes[1].Text)
	require.NotContains(p.t, msgs[1].Content, "private-message-id")
	return &providers.LLMResponse{Content: fmt.Sprintf(`[
		{"kind":"preference","key":"style","value":"中文简洁","evidence":%d},
		{"kind":"record","key":"progress.genshin","value":"原神主线打到第三章","date":"2026-05-21","category":"日常/游戏","aliases":["提瓦特进度"],"evidence":%d}
	]`, in.Notes[0].ID, in.Notes[1].ID)}, nil
}

func TestPrivateMemoryReviewRunsOffTurnAndProcessesEachBatchOnce(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = t.TempDir()
	cfg.Agents.Defaults.ModelName = "review-test"
	require.NoError(
		t,
		json.Unmarshal(
			[]byte(`{"weixin":{"type":"weixin","enabled":true,"settings":{"account_id":"bot"}}}`),
			&cfg.Channels,
		),
	)
	p := &reviewTestProvider{t: t}
	msgBus := bus.NewMessageBus()
	defer msgBus.Close()
	al := NewAgentLoop(cfg, msgBus, p)
	defer al.Close()
	al.channelManager = &memoryReviewTestChannels{}
	process := func(text, typ, authored string) string {
		t.Helper()
		res, processErr := al.processMessage(t.Context(), bus.InboundMessage{
			Context: bus.InboundContext{
				Channel:   "weixin",
				ChatID:    "test-" + typ,
				ChatType:  typ,
				MessageID: "private-message-id",
			},
			Content:     text,
			Sender:      bus.SenderInfo{Platform: "weixin", PlatformID: "alice"},
			PrivateChat: &bus.PrivateChat{Text: authored},
		})
		require.NoError(t, processErr)
		return res
	}
	for _, text := range []string{"我平时喜欢用中文简短回答", "我在2026-05-21把原神打到第三章"} {
		require.Equal(t, "收到", process(text, "direct", text))
	}
	process("这里是群聊", "group", "这里是群聊")
	process("不可识别的引述", "direct", "")
	require.Zero(t, p.reviews)
	require.Contains(t, process("/memory export", "direct", "/memory export"), "没有更多记录")
	db := al.GetRegistry().GetDefaultAgent().memoryDB
	now := time.Now()
	keys, err := db.DueReviews(t.Context(), now, now)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	batch, err := db.ReadReview(t.Context(), keys[0])
	require.NoError(t, err)
	require.Len(t, batch.Notes, 2)
	due := batch.Notes[1].At.Add(2 * time.Minute)
	al.reviewDueMemory(t.Context(), due.Add(-time.Millisecond), map[string]time.Time{})
	require.Zero(t, p.reviews)
	al.reviewDueMemory(t.Context(), due, map[string]time.Time{})
	require.Equal(t, 1, p.reviews)
	export := process("/memory export", "direct", "/memory export")
	var record memory.Record
	require.NoError(t, json.NewDecoder(strings.NewReader(export)).Decode(&record))
	require.Equal(t, "2026-05-21", record.Date)
	require.Equal(t, "日常/游戏", record.Category)
	require.Contains(t, record.Aliases, "提瓦特进度")
	require.Contains(t, process("/preference list", "direct", "/preference list"), "中文简洁")
	select {
	case out := <-msgBus.OutboundChan():
		require.Contains(t, out.Content, "更新 1 条偏好、1 条永久记录")
		require.Equal(t, "test-direct", out.Context.ChatID)
	default:
		t.Fatal("expected a review receipt")
	}
	al.reviewDueMemory(t.Context(), time.Now().Add(time.Hour), map[string]time.Time{})
	require.Equal(t, 1, p.reviews)
}

type reviewBackoffProvider struct {
	failing, healthy int
}

func (*reviewBackoffProvider) GetDefaultModel() string { return "review-test" }

func (p *reviewBackoffProvider) Chat(
	_ context.Context, msgs []providers.Message, _ []providers.ToolDefinition, _ string, _ map[string]any,
) (*providers.LLMResponse, error) {
	if strings.Contains(msgs[1].Content, "healthy evidence") {
		p.healthy++
		return &providers.LLMResponse{Content: "[]"}, nil
	}
	p.failing++
	return nil, errors.New("simulated review failure")
}

func TestMemoryReviewBackoffDoesNotBlockLaterSessions(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = t.TempDir()
	cfg.Agents.Defaults.ModelName = "review-test"
	p := &reviewBackoffProvider{}
	msgBus := bus.NewMessageBus()
	defer msgBus.Close()
	al := NewAgentLoop(cfg, msgBus, p)
	defer al.Close()
	db := al.GetRegistry().GetDefaultAgent().memoryDB
	ctx := t.Context()
	queue := func(session, evidence string) {
		t.Helper()
		_, err := db.AppendTurn(ctx, memory.SessionWrite{
			Key: session, Private: true,
			Review: &memory.ReviewNote{Text: evidence, Source: "fixture"},
		}, []providers.Message{{Role: "user", Content: evidence}})
		require.NoError(t, err)
	}
	for i := range 16 {
		queue(fmt.Sprintf("failing-%d", i), "failing evidence")
	}
	queue("healthy", "healthy evidence")
	now := time.Now().Add(3 * time.Minute)
	retryAfter := make(map[string]time.Time)

	al.reviewDueMemory(ctx, now, retryAfter)
	require.Equal(t, 16, p.failing)
	require.Zero(t, p.healthy)
	require.Len(t, retryAfter, 16)

	al.reviewDueMemory(ctx, now.Add(memoryReviewInterval), retryAfter)
	require.Equal(t, 16, p.failing, "sessions in backoff should not retry")
	require.Equal(t, 1, p.healthy, "the later session must get a review slot")
	healthy, err := db.ReadReview(ctx, "healthy")
	require.NoError(t, err)
	require.Empty(t, healthy.Notes)

	al.reviewDueMemory(ctx, now.Add(5*time.Minute), retryAfter)
	require.Equal(t, 32, p.failing, "sessions become eligible again when backoff expires")
	require.Equal(t, 1, p.healthy)
}
