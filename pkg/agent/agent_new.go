package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/commands"
)

// A busy session must reject /new before it can enter the steering queue.
func (al *AgentLoop) tryRejectNewCommand(
	ctx context.Context,
	msg bus.InboundMessage,
	sessionKey string,
	active *turnState,
) bool {
	if name, ok := commands.CommandName(msg.Content); !ok || name != "new" {
		return false
	}

	taskName := []rune(strings.Join(strings.Fields(active.snapshot().UserMessage), " "))
	if len(taskName) == 0 {
		taskName = []rune("当前任务")
	} else if len(taskName) > 80 {
		taskName = append(taskName[:77], '.', '.', '.')
	}
	// This is a separate command reply, not the running task's final answer.
	// Bypass response deduplication and leave its stream/placeholder intact.
	outboundCtx := *cloneInboundContext(&msg.Context)
	outboundCtx.ReplyToMessageID = msg.MessageID
	if outboundCtx.Raw == nil {
		outboundCtx.Raw = make(map[string]string)
	}
	outboundCtx.Raw[metadataKeyOutboundKind] = outboundKindCommandReply
	al.bus.PublishOutbound(ctx, bus.OutboundMessage{
		Context:    outboundCtx,
		SessionKey: sessionKey,
		Content:    fmt.Sprintf("当前有个「%s」任务在执行，请待会再试。", string(taskName)),
	})
	return true
}
