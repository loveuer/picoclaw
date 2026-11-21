// PicoClaw - Ultra-lightweight personal AI agent

package agent

import (
	"context"

	"github.com/sipeed/picoclaw/pkg/bus"
	runtimeevents "github.com/sipeed/picoclaw/pkg/events"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// Finalize handles turn finalization, either:
// It commits the remaining session messages for both direct answers and tool deliveries.
func (p *Pipeline) Finalize(
	ctx context.Context,
	turnCtx context.Context,
	ts *turnState,
	exec *turnExecution,
	turnStatus TurnEndStatus,
	finalContent string,
) (turnResult, error) {
	al := p.al

	if ts.hardAbortRequested() {
		return al.abortTurn(ts)
	}
	management := ts.opts.memoryManagement
	if management != nil {
		if management.needsChoice() {
			finalContent = management.choiceQuestion()
		} else {
			finalContent = management.receipt()
		}
	}
	ts.setPhase(TurnPhaseFinalizing)
	ts.setFinalContent(finalContent)
	if !ts.opts.NoHistory {
		finalMsg := providers.Message{
			Role: "assistant", Content: finalContent, ModelName: exec.llmModelName,
			ReasoningContent: responseReasoningContent(exec.response),
		}
		if exec.allResponsesHandled {
			finalMsg = providers.Message{
				Role:        "assistant",
				Content:     handledToolResponseSummary,
				Attachments: exec.handledAttachments,
			}
		}
		ts.addTurnMessage(finalMsg)
		if turnStatus == TurnEndStatusCompleted && ts.turnMemory != nil {
			ts.turnMemory.write.Review = reviewNote(ts.opts)
		}
		if err := ts.commitTurnMemory(turnCtx, false); err != nil {
			if ts.hardAbortRequested() {
				return al.abortTurn(ts)
			}
			al.emitEvent(
				runtimeevents.KindAgentError,
				ts.eventMeta("runTurn", "turn.error"),
				ErrorPayload{
					Stage:   "session_save",
					Message: err.Error(),
				},
			)
			cancelConfiguredStreamingLLM(turnCtx, exec)
			return turnResult{status: TurnEndStatusError}, err
		}
	}

	ts.historyCommitted.Store(true)
	if management != nil && (management.needsChoice() || management.correction != nil) && ts.turnMemory != nil {
		management.revision = ts.turnMemory.write.Revision
		al.storeMemoryChoice(ts.sessionKey, management)
	}
	if !ts.opts.NoHistory && ts.opts.EnableSummary {
		al.contextManager.Compact(
			turnCtx,
			&CompactRequest{
				SessionKey: ts.sessionKey,
				Reason:     ContextCompressReasonSummarize,
				Budget:     ts.agent.ContextWindow,
			},
		)
	}

	if exec.allResponsesHandled {
		ts.setPhase(TurnPhaseCompleted)
		return turnResult{
			finalContent: finalContent, modelName: exec.llmModelName, status: turnStatus,
			followUps: append([]bus.InboundMessage(nil), ts.followUps...),
		}, nil
	}

	contextUsage := computeContextUsage(ts.agent, ts.sessionKey)
	streamErr := finalizeConfiguredStreamingLLM(turnCtx, ts, exec, finalContent, contextUsage)
	// If streaming never became visible, publish the fallback answer
	// so the final answer is still delivered outside normal SendResponse.
	if ((streamErr != nil && !isConfiguredStreamingVisibleError(streamErr)) || exec.streamingFallback) &&
		!ts.opts.SendResponse && ts.opts.AllowStreaming && finalContent != "" {
		msg := outboundMessageForTurnWithOptions(ts, finalContent, outboundTurnMessageOptions{
			modelName: exec.llmModelName,
		})
		msg.ContextUsage = contextUsage
		markFinalOutbound(&msg)
		_ = al.bus.PublishOutbound(turnCtx, msg)
	}
	if streamErr != nil && isConfiguredStreamingVisibleError(streamErr) {
		ts.setPhase(TurnPhaseCompleted)
		return turnResult{
			finalContent: finalContent,
			status:       TurnEndStatusError,
			followUps:    append([]bus.InboundMessage(nil), ts.followUps...),
		}, streamErr
	}
	ts.setPhase(TurnPhaseCompleted)
	return turnResult{
		finalContent: finalContent,
		modelName:    exec.llmModelName,
		status:       turnStatus,
		followUps:    append([]bus.InboundMessage(nil), ts.followUps...),
	}, nil
}
