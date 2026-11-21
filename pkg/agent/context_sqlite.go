package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	runtimeevents "github.com/sipeed/picoclaw/pkg/events"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tokenizer"
)

// sqliteContextManager assembles SQLite sessions and compacts complete turns.
type sqliteContextManager struct {
	al          *AgentLoop
	summarizing sync.Map // dedup for async Compact (post-turn)
	contextMu   sync.Mutex
}

type CompactRequest struct {
	SessionKey string
	Reason     ContextCompressReason
	Budget     int
}

func (m *sqliteContextManager) beginTurn(ctx context.Context, ts *turnState) (memory.SessionSnapshot, error) {
	m.contextMu.Lock()
	defer m.contextMu.Unlock()
	return ts.beginTurnMemory(ctx, ts.agent.turnStore)
}

func (m *sqliteContextManager) Compact(_ context.Context, req *CompactRequest) error {
	switch req.Reason {
	case ContextCompressReasonProactive, ContextCompressReasonRetry:
		// Sync emergency compression — budget exceeded.
		if result, ok := m.forceCompression(req.SessionKey); ok {
			m.al.emitEvent(
				runtimeevents.KindAgentContextCompress,
				m.al.newTurnEventScope("", req.SessionKey, nil).meta(0, "forceCompression", "turn.context.compress"),
				ContextCompressPayload{
					Reason:            req.Reason,
					DroppedMessages:   result.DroppedMessages,
					RemainingMessages: result.RemainingMessages,
				},
			)
		}
	case ContextCompressReasonSummarize:
		m.maybeSummarize(req.SessionKey)
	}
	return nil
}

func (m *sqliteContextManager) clearOwned(
	ctx context.Context,
	store turnSessionStore,
	opts processOptions,
) error {
	if store == nil {
		return fmt.Errorf("sessions not initialized")
	}
	write, err := newSessionWrite(opts)
	if err != nil {
		return err
	}
	m.contextMu.Lock()
	defer m.contextMu.Unlock()
	return store.ClearSession(ctx, write)
}

// maybeSummarize triggers summarization if the session history exceeds thresholds.
// It runs asynchronously in a goroutine.
func (m *sqliteContextManager) maybeSummarize(sessionKey string) {
	agent := m.al.agentForSession(sessionKey)
	if agent == nil {
		return
	}

	newHistory := agent.Sessions.GetHistory(sessionKey)
	tokenEstimate := m.estimateTokens(newHistory)
	threshold := agent.ContextWindow * agent.SummarizeTokenPercent / 100

	if len(newHistory) > agent.SummarizeMessageThreshold || tokenEstimate > threshold {
		summarizeKey := agent.ID + ":" + sessionKey
		if _, loading := m.summarizing.LoadOrStore(summarizeKey, true); !loading {
			go func() {
				defer m.summarizing.Delete(summarizeKey)
				defer func() {
					if r := recover(); r != nil {
						logger.WarnCF("agent", "Summarization panic recovered", map[string]any{
							"session_key": sessionKey,
							"panic":       r,
						})
					}
				}()
				logger.Debug("Memory threshold reached. Optimizing conversation history...")
				m.summarizeSession(agent, sessionKey)
			}()
		}
	}
}

type compressionResult struct {
	DroppedMessages   int
	RemainingMessages int
}

// forceCompression aggressively reduces context when the limit is hit.
// It drops the oldest ~50% of Turns (a Turn is a complete user→LLM→response
// cycle, as defined in #1316), so tool-call sequences are never split.
func (m *sqliteContextManager) forceCompression(sessionKey string) (compressionResult, bool) {
	agent := m.al.agentForSession(sessionKey)
	if agent == nil {
		return compressionResult{}, false
	}

	db := agent.turnStore
	if db == nil {
		return compressionResult{}, false
	}
	snap, err := db.ReadSession(context.Background(), sessionKey)
	if err != nil {
		return compressionResult{}, false
	}
	history := snap.Messages
	if len(history) <= 2 {
		return compressionResult{}, false
	}

	turns := parseTurnBoundaries(history)
	var mid int
	if len(turns) >= 2 {
		mid = turns[len(turns)/2]
	} else {
		mid = findSafeBoundary(history, len(history)/2)
	}
	if mid <= 0 || mid >= len(history) || !isSafeBoundary(history, mid) {
		return compressionResult{}, false
	}
	droppedCount := mid
	keptCount := len(history) - mid

	compressionNote := fmt.Sprintf(
		"[Emergency compression dropped %d oldest messages due to context limit]",
		droppedCount,
	)
	if snap.Summary != "" {
		compressionNote = snap.Summary + "\n\n" + compressionNote
	}
	applied, err := db.CompactPrefix(
		context.Background(), sessionKey, snap.Revision, snap.RowIDs[mid-1], compressionNote,
	)
	if err != nil || !applied {
		return compressionResult{}, false
	}

	logger.WarnCF("agent", "Forced compression executed", map[string]any{
		"session_key":  sessionKey,
		"dropped_msgs": droppedCount,
		"new_count":    keptCount,
	})

	return compressionResult{
		DroppedMessages:   droppedCount,
		RemainingMessages: keptCount,
	}, true
}

func (m *sqliteContextManager) summarizeSession(agent *AgentInstance, sessionKey string) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	db := agent.turnStore
	if db == nil {
		return
	}
	snap, err := db.ReadSession(ctx, sessionKey)
	if err != nil {
		return
	}
	history, summary := snap.Messages, snap.Summary

	if len(history) <= 4 {
		return
	}

	safeCut := findSafeBoundary(history, len(history)-4)
	if safeCut <= 0 || !isSafeBoundary(history, safeCut) {
		return
	}
	keepCount := len(history) - safeCut
	toSummarize := history[:safeCut]

	maxMessageTokens := agent.ContextWindow / 2
	validMessages := make([]providers.Message, 0)
	omitted := false

	for _, msg := range toSummarize {
		if msg.Role != "user" && msg.Role != "assistant" {
			continue
		}
		msgTokens := len(msg.Content) / 2
		if msgTokens > maxMessageTokens {
			omitted = true
			continue
		}
		validMessages = append(validMessages, msg)
	}

	if len(validMessages) == 0 {
		return
	}

	const (
		maxSummarizationMessages = 10
		llmMaxRetries            = 3
	)

	var finalSummary string
	if len(validMessages) > maxSummarizationMessages {
		mid := len(validMessages) / 2
		mid = m.findNearestUserMessage(validMessages, mid)

		part1 := validMessages[:mid]
		part2 := validMessages[mid:]

		s1, _ := m.summarizeBatch(ctx, agent, part1, "")
		s2, _ := m.summarizeBatch(ctx, agent, part2, "")

		mergePrompt := fmt.Sprintf(
			"Merge these two conversation summaries into one cohesive summary:\n\n1: %s\n\n2: %s",
			s1, s2,
		)

		resp, err := m.retryLLMCall(ctx, agent, mergePrompt, llmMaxRetries)
		if err == nil && resp.Content != "" {
			finalSummary = resp.Content
		} else {
			finalSummary = s1 + " " + s2
		}
	} else {
		finalSummary, _ = m.summarizeBatch(ctx, agent, validMessages, summary)
	}

	if omitted && finalSummary != "" {
		finalSummary += "\n[Note: Some oversized messages were omitted from this summary for efficiency.]"
	}

	if finalSummary != "" {
		m.contextMu.Lock()
		if active, ok := m.al.activeTurnStates.Load(sessionKey); ok {
			if ts, ok := active.(*turnState); ok && !ts.historyCommitted.Load() {
				m.contextMu.Unlock()
				return
			}
		}
		applied, err := db.CompactPrefix(ctx, sessionKey, snap.Revision, snap.RowIDs[safeCut-1], finalSummary)
		if err != nil || !applied {
			m.contextMu.Unlock()
			return
		}

		m.contextMu.Unlock()
		m.al.emitEvent(
			runtimeevents.KindAgentSessionSummarize,
			m.al.newTurnEventScope(agent.ID, sessionKey, nil).meta(0, "summarizeSession", "turn.session.summarize"),
			SessionSummarizePayload{
				SummarizedMessages: len(validMessages),
				KeptMessages:       keepCount,
				SummaryLen:         len(finalSummary),
				OmittedOversized:   omitted,
			},
		)
	}
}

func (m *sqliteContextManager) findNearestUserMessage(messages []providers.Message, mid int) int {
	originalMid := mid

	for mid > 0 && messages[mid].Role != "user" {
		mid--
	}

	if messages[mid].Role == "user" {
		return mid
	}

	mid = originalMid
	for mid < len(messages) && messages[mid].Role != "user" {
		mid++
	}

	if mid < len(messages) {
		return mid
	}

	return originalMid
}

func (m *sqliteContextManager) retryLLMCall(
	ctx context.Context,
	agent *AgentInstance,
	prompt string,
	maxRetries int,
) (*providers.LLMResponse, error) {
	const llmTemperature = 0.3

	var resp *providers.LLMResponse
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		m.al.activeRequestsInc()
		resp, err = func() (*providers.LLMResponse, error) {
			defer m.al.activeRequestsDec()
			return agent.Provider.Chat(
				ctx,
				[]providers.Message{{Role: "user", Content: prompt}},
				nil,
				agent.Model,
				map[string]any{
					"max_tokens":       min(1024, max(1, agent.MaxTokens)),
					"temperature":      llmTemperature,
					"prompt_cache_key": agent.ID,
				},
			)
		}()

		if err == nil && resp != nil && resp.Content != "" {
			return resp, nil
		}
		if attempt < maxRetries-1 {
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
		}
	}

	return resp, err
}

func (m *sqliteContextManager) summarizeBatch(
	ctx context.Context,
	agent *AgentInstance,
	batch []providers.Message,
	existingSummary string,
) (string, error) {
	const (
		llmMaxRetries             = 3
		fallbackMinContentLength  = 200
		fallbackMaxContentPercent = 10
	)

	var sb strings.Builder
	sb.WriteString("Provide a concise summary of this conversation segment, preserving core context and key points.\n")
	if existingSummary != "" {
		sb.WriteString("Existing context: ")
		sb.WriteString(existingSummary)
		sb.WriteString("\n")
	}
	sb.WriteString("\nCONVERSATION:\n")
	for _, msg := range batch {
		fmt.Fprintf(&sb, "%s: %s\n", msg.Role, msg.Content)
	}
	prompt := sb.String()

	response, err := m.retryLLMCall(ctx, agent, prompt, llmMaxRetries)
	if err == nil && response.Content != "" {
		return strings.TrimSpace(response.Content), nil
	}

	var fallback strings.Builder
	fallback.WriteString("Conversation summary: ")
	for i, msg := range batch {
		if i > 0 {
			fallback.WriteString(" | ")
		}
		content := strings.TrimSpace(msg.Content)
		runes := []rune(content)
		if len(runes) == 0 {
			fallback.WriteString(fmt.Sprintf("%s: ", msg.Role))
			continue
		}

		keepLength := len(runes) * fallbackMaxContentPercent / 100
		if keepLength < fallbackMinContentLength {
			keepLength = fallbackMinContentLength
		}
		if keepLength > len(runes) {
			keepLength = len(runes)
		}

		content = string(runes[:keepLength])
		if keepLength < len(runes) {
			content += "..."
		}
		fallback.WriteString(fmt.Sprintf("%s: %s", msg.Role, content))
	}
	return fallback.String(), nil
}

func (m *sqliteContextManager) estimateTokens(messages []providers.Message) int {
	total := 0
	for _, msg := range messages {
		total += EstimateMessageTokens(msg)
	}
	return total
}

func boundedSessionSummary(summary string, budget, reserved int) string {
	limit := 2048
	if budget > 0 {
		limit = min(limit, max(0, budget-reserved)/4)
	}
	if summary == "" || limit == 0 {
		return ""
	}
	if tokenizer.EstimateMessageTokens(providers.Message{Role: "system", Content: summary}) <= limit {
		return summary
	}
	runes := []rune(summary)
	low, high := 0, len(runes)
	for low < high {
		mid := (low + high + 1) / 2
		if tokenizer.EstimateMessageTokens(providers.Message{Role: "system", Content: string(runes[:mid])}) <= limit {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return string(runes[:low])
}
