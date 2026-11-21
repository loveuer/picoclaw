package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/providers"
)

const (
	checkpointBatchLimit = 8
	checkpointByteLimit  = 512 << 10
)

// turnSessionStore is the transaction boundary used by durable turns.
type turnSessionStore interface {
	TurnSnapshot(context.Context, string, bool) (string, memory.SessionSnapshot, error)
	AppendTurn(context.Context, memory.SessionWrite, []providers.Message) (memory.MessageSpan, error)
	RemoveTurnSpans(context.Context, memory.SessionWrite, []memory.MessageSpan) error
	ClearSession(context.Context, memory.SessionWrite) error
	ReadSession(context.Context, string) (memory.SessionSnapshot, error)
	CompactPrefix(context.Context, string, int64, int64, string) (bool, error)
}

type turnMemory struct {
	store   turnSessionStore
	write   memory.SessionWrite
	pending []providers.Message
	spans   []memory.MessageSpan
	batches int
	bytes   int
}

func newSessionWrite(opts processOptions) (memory.SessionWrite, error) {
	w := memory.SessionWrite{
		Key:     opts.Dispatch.SessionKey,
		Private: opts.privateMemory,
	}
	if scope := opts.Dispatch.SessionScope; scope != nil {
		b, err := json.Marshal(scope)
		if err != nil {
			return w, err
		}
		w.Scope = b
		w.Aliases = opts.Dispatch.SessionAliases
	}
	return w, nil
}

func (ts *turnState) beginTurnMemory(
	ctx context.Context,
	store turnSessionStore,
) (memory.SessionSnapshot, error) {
	if store == nil {
		return memory.SessionSnapshot{}, fmt.Errorf("sessions not initialized")
	}
	w, err := newSessionWrite(ts.opts)
	if err != nil {
		return memory.SessionSnapshot{}, err
	}
	key, snap, err := store.TurnSnapshot(ctx, ts.sessionKey, w.Private)
	if err != nil {
		return snap, err
	}
	w.Key, w.Revision = key, snap.Revision
	ts.turnMemory = &turnMemory{store: store, write: w}
	return snap, nil
}

func (ts *turnState) addTurnMessage(msg providers.Message) {
	mem := ts.turnMemory
	if mem == nil {
		return
	}
	if msg.CreatedAt == nil {
		now := time.Now().UTC()
		msg.CreatedAt = &now
	}
	// Count serialized message bytes, including model arguments and attachments.
	if b, err := json.Marshal(msg); err == nil {
		mem.bytes += len(b)
	}
	mem.pending = append(mem.pending, msg)
}

func (ts *turnState) commitTurnMemory(ctx context.Context, checkpoint bool) error {
	mem := ts.turnMemory
	if mem == nil {
		return nil
	}
	if checkpoint {
		mem.batches++
		if mem.batches < checkpointBatchLimit && mem.bytes < checkpointByteLimit {
			return nil
		}
	}
	if len(mem.pending) == 0 {
		return nil
	}
	if !completeToolBatches(mem.pending) {
		if checkpoint {
			return nil
		}
		return fmt.Errorf("refusing to save a turn with incomplete tool results")
	}
	span, err := mem.store.AppendTurn(ctx, mem.write, mem.pending)
	if err != nil {
		return err
	}
	mem.write.Revision++
	mem.write.Scope, mem.write.Aliases = nil, nil
	mem.spans = append(mem.spans, span)
	mem.pending = nil
	mem.batches, mem.bytes = 0, 0
	return nil
}

// A safe batch has exactly the results corresponding to each assistant
// message, before any subsequent non-tool message. Reused IDs are scoped to
// the assistant's own batch.
func completeToolBatches(msgs []providers.Message) bool {
	var open map[string]struct{}
	for _, m := range msgs {
		if m.Role == "tool" {
			if _, ok := open[m.ToolCallID]; !ok {
				return false
			}
			delete(open, m.ToolCallID)
			continue
		}
		if len(open) != 0 {
			return false
		}
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			open = make(map[string]struct{}, len(m.ToolCalls))
			for _, call := range m.ToolCalls {
				if call.ID == "" {
					return false
				}
				if _, ok := open[call.ID]; ok {
					return false
				}
				open[call.ID] = struct{}{}
			}
		}
	}
	return len(open) == 0
}
