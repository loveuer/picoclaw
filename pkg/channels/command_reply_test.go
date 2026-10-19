package channels

import (
	"context"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
)

func TestPreSend_CommandReplyPreservesRunningTurn(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "placeholder", true: "stream"}[streaming], func(t *testing.T) {
			m := newTestManager()
			m.RecordPlaceholder("test", "123", "placeholder-1")
			m.RecordTypingStop("test", "123", func() { t.Error("command reply stopped typing") })
			m.reactionUndos.Store("test:123", reactionEntry{undo: func() { t.Error("command reply undid reaction") }})
			streamKey := streamSuppressionKey("test", "123", "session-1")
			if streaming {
				m.streamActive.Store(streamKey, true)
				m.streamAuxiliaryTombstones.Store(streamKey, time.Now())
			}
			ch := &mockChannel{}
			msg := testOutboundMessage(bus.OutboundMessage{
				Context: bus.InboundContext{Channel: "test", ChatID: "123", Raw: map[string]string{
					"outbound_kind": "command_reply",
				}},
				SessionKey: "session-1",
				Content:    "当前有个任务在执行，请待会再试。",
			})
			if _, handled := m.preSend(context.Background(), "test", msg, ch); handled {
				t.Fatal("command reply should be sent as a separate message")
			}
			if _, ok := m.placeholders.Load("test:123"); !ok || ch.editedMessages != 0 {
				t.Fatal("command reply consumed the running turn's placeholder")
			}
			if _, ok := m.typingStops.Load("test:123"); !ok {
				t.Fatal("command reply consumed the typing state")
			}
			if _, ok := m.reactionUndos.Load("test:123"); !ok {
				t.Fatal("command reply consumed the reaction state")
			}
			if streaming {
				if _, ok := m.streamActive.Load(streamKey); !ok || !m.streamAuxiliaryTombstoneActive(streamKey) {
					t.Fatal("command reply changed stream finalization state")
				}
			}
		})
	}
}

func TestBaseChannel_NewDoesNotReplaceRunningIndicators(t *testing.T) {
	msgBus := bus.NewMessageBus()
	ch := &mockChannel{BaseChannel: *NewBaseChannel("test", nil, msgBus, []string{"*"})}
	m := newTestManager()
	ch.SetOwner(ch)
	ch.SetPlaceholderRecorder(m)
	m.RecordPlaceholder("test", "123", "original-placeholder")
	for _, content := range []string{"/new", "/new 新问题", "/NEW@bot\t新问题"} {
		if err := ch.HandleMessageWithContext(t.Context(), "123", content, nil, bus.InboundContext{
			ChatID: "123", SenderID: "user1",
		}); err != nil {
			t.Fatal(err)
		}
		select {
		case msg := <-msgBus.InboundChan():
			if msg.Content != content {
				t.Fatalf("forwarded content = %q", msg.Content)
			}
		default:
			t.Fatal("command not forwarded")
		}
	}
	if ch.placeholdersSent != 0 {
		t.Fatal("/new created a replacement placeholder")
	}
	v, ok := m.placeholders.Load("test:123")
	if !ok || v.(placeholderEntry).id != "original-placeholder" {
		t.Fatal("original placeholder was replaced")
	}
}
