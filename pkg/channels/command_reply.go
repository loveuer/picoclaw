package channels

import "github.com/sipeed/picoclaw/pkg/bus"

// IsCommandReply identifies a reply that must leave the running task's
// placeholder, stream, and tool feedback messages intact.
func IsCommandReply(msg bus.OutboundMessage) bool {
	return msg.Context.Raw["outbound_kind"] == "command_reply"
}
