// PicoClaw - Ultra-lightweight personal AI agent

package agent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/constants"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/routing"
	"github.com/sipeed/picoclaw/pkg/session"
	"github.com/sipeed/picoclaw/pkg/utils"
)

func (al *AgentLoop) buildContinuationTarget(msg bus.InboundMessage) (*continuationTarget, error) {
	if msg.Channel == "system" {
		return nil, nil
	}

	route, _, err := al.resolveMessageRoute(msg)
	if err != nil {
		return nil, err
	}
	allocation := al.allocateRouteSession(route, msg)

	return &continuationTarget{
		SessionKey: inboundSessionKey(allocation, msg),
		Channel:    msg.Channel,
		ChatID:     msg.ChatID,
	}, nil
}

func (al *AgentLoop) ProcessDirect(
	ctx context.Context,
	content, sessionKey string,
) (string, error) {
	return al.processLocalPrompt(ctx, content, sessionKey, "cli", "direct", false)
}

// ProcessLocalCLI is the trusted operator entry point for the instance owner.
// Synthetic and inbound turns do not inherit this authority.
func (al *AgentLoop) ProcessLocalCLI(ctx context.Context, content, sessionKey string) (string, error) {
	return al.processLocalPrompt(ctx, content, sessionKey, "cli", "direct", true)
}

func (al *AgentLoop) ProcessDirectWithChannel(
	ctx context.Context,
	content, sessionKey, channel, chatID string,
) (string, error) {
	return al.processLocalPrompt(ctx, content, sessionKey, channel, chatID, false)
}

func (al *AgentLoop) processLocalPrompt(
	ctx context.Context, content, sessionKey, channel, chatID string, localPrivate bool,
) (string, error) {
	if err := al.ensureHooksInitialized(ctx); err != nil {
		return "", err
	}
	if err := al.ensureMCPInitialized(ctx); err != nil {
		return "", err
	}

	msg := bus.InboundMessage{
		Context: bus.InboundContext{
			Channel:  channel,
			ChatID:   chatID,
			ChatType: "direct",
			SenderID: "cron",
		},
		Content:    content,
		SessionKey: sessionKey,
	}

	return al.processMessageWithLocalAccess(ctx, msg, localPrivate)
}

func (al *AgentLoop) ProcessHeartbeat(
	ctx context.Context,
	content, channel, chatID string,
) (string, error) {
	if err := al.ensureHooksInitialized(ctx); err != nil {
		return "", err
	}
	if err := al.ensureMCPInitialized(ctx); err != nil {
		return "", err
	}

	agent := al.GetRegistry().GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent for heartbeat")
	}
	dispatch := DispatchRequest{
		SessionKey:  "heartbeat",
		UserMessage: content,
	}
	if channel != "" || chatID != "" {
		dispatch.InboundContext = &bus.InboundContext{
			Channel:  channel,
			ChatID:   chatID,
			ChatType: "direct",
			SenderID: "heartbeat",
		}
	}
	return al.runAgentLoop(ctx, agent, processOptions{
		Dispatch:             dispatch,
		DefaultResponse:      defaultResponse,
		EnableSummary:        false,
		SendResponse:         false,
		SuppressToolFeedback: true,
		NoHistory:            true, // Don't load session history for heartbeat
	})
}

func (al *AgentLoop) prepareInboundMessageForAgent(
	ctx context.Context,
	msg bus.InboundMessage,
) bus.InboundMessage {
	msg = bus.NormalizeInboundMessage(msg)

	var hadAudio bool
	msg, hadAudio = al.transcribeAudioInMessage(ctx, msg)

	// For audio messages the placeholder was deferred by the channel.
	// Now that transcription (and optional feedback) is done, send it.
	if hadAudio && al.channelManager != nil {
		al.channelManager.SendPlaceholder(ctx, msg.Channel, msg.ChatID)
	}

	return msg
}

func (al *AgentLoop) processMessage(ctx context.Context, msg bus.InboundMessage) (string, error) {
	return al.processMessageWithLocalAccess(ctx, msg, false)
}

func (al *AgentLoop) processMessageWithLocalAccess(
	ctx context.Context, msg bus.InboundMessage, localPrivate bool,
) (string, error) {
	msg = al.prepareInboundMessageForAgent(ctx, msg)

	// Add message preview to log (show full content for error messages)
	var logContent string
	if isPrivateChat(msg) || localPrivate {
		logContent = "[private message]"
	} else if strings.Contains(msg.Content, "Error:") || strings.Contains(msg.Content, "error") {
		logContent = msg.Content // Full content for errors
	} else {
		logContent = utils.Truncate(msg.Content, 80)
	}
	logger.InfoCF(
		"agent",
		fmt.Sprintf("Processing message from %s:%s: %s", msg.Channel, msg.SenderID, logContent),
		map[string]any{
			"channel":     msg.Channel,
			"chat_id":     msg.ChatID,
			"sender_id":   msg.SenderID,
			"session_key": msg.SessionKey,
		},
	)

	// Route system messages to processSystemMessage
	if msg.Channel == "system" {
		return al.processSystemMessage(ctx, msg)
	}

	route, agent, routeErr := al.resolveMessageRoute(msg)
	if routeErr != nil {
		return "", routeErr
	}

	allocation := al.allocateRouteSession(route, msg)

	// Resolve session key from the route allocation, while preserving explicit
	// agent-scoped keys supplied by the caller.
	scopeKey := inboundSessionKey(allocation, msg)
	private := localPrivate || isPrivateChat(msg)
	var aliases []string
	var authored, source string
	if localPrivate {
		// The caller's label is only a label, never an override into an IM session.
		allocation.Scope = session.SessionScope{
			Version: session.ScopeVersionV1, AgentID: agent.ID, Channel: "cli",
			Dimensions: []string{"chat"},
			Values:     map[string]string{"chat": fmt.Sprintf("local:%x", sha256.Sum256([]byte(msg.SessionKey)))},
		}
		scopeKey = session.BuildSessionKey(allocation.Scope)
		authored, source = msg.Content, "cli session "+scopeKey
	} else if private {
		authored = msg.PrivateChat.Text
		source = fmt.Sprintf("%s message %s", msg.Context.Channel, msg.Context.MessageID)
	} else {
		aliases = buildSessionAliases(scopeKey, append(allocation.SessionAliases, msg.SessionKey)...)
	}

	sessionKey := scopeKey
	if agent.memoryErr != nil {
		return "", agent.memoryErr
	}
	if err := authorizeMemorySession(ctx, agent, sessionKey, private); err != nil {
		return "", err
	}
	if answer, handled, err := agent.handlePrivateMemory(
		ctx,
		private,
		msg.Content,
		authored,
		source,
	); handled {
		return answer, err
	}

	// Reset message-tool state for this round so we don't skip publishing due to a previous round.
	if tool, ok := agent.Tools.Get("message"); ok {
		if resetter, ok := tool.(interface{ ResetSentInRound(sessionKey string) }); ok {
			resetter.ResetSentInRound(sessionKey)
		}
	}

	logger.InfoCF("agent", "Routed message",
		map[string]any{
			"agent_id":           agent.ID,
			"scope_key":          scopeKey,
			"session_key":        sessionKey,
			"matched_by":         route.MatchedBy,
			"route_agent":        route.AgentID,
			"route_channel":      route.Channel,
			"route_main_session": allocation.MainSessionKey,
		})

	cfg := al.GetConfig()
	opts := processOptions{
		memoryAutoCapture:  cfg != nil && !cfg.Memory.DisableAutoCapture,
		privateMemory:      private,
		memorySender:       msg.Sender,
		memoryAuthoredText: authored,
		memorySource:       source,
		Dispatch: DispatchRequest{
			SessionKey:     sessionKey,
			SessionAliases: aliases,
			InboundContext: cloneInboundContext(&msg.Context),
			RouteResult:    cloneResolvedRoute(&route),
			SessionScope:   session.CloneScope(&allocation.Scope),
			UserMessage:    msg.Content,
			Media:          append([]string(nil), msg.Media...),
		},
		SenderID:          msg.SenderID,
		SenderDisplayName: msg.Sender.DisplayName,
		DefaultResponse:   defaultResponse,
		EnableSummary:     true,
		SendResponse:      false,
		AllowStreaming:    true,
	}
	var err error
	opts, err = resolveTurnProfileOptions(al.GetConfig(), opts)
	if err != nil {
		return "", err
	}

	// context-dependent commands check their own Runtime fields and report
	// "unavailable" when the required capability is nil.
	if response, handled := al.handleCommand(ctx, msg, agent, &opts); handled {
		return response, nil
	}

	return al.runMessagePrompt(ctx, agent, opts)
}

func (al *AgentLoop) runMessagePrompt(ctx context.Context, agent *AgentInstance, opts processOptions) (string, error) {
	management, reply, err := al.prepareMemoryManagement(ctx, agent, opts)
	if err != nil || reply != "" {
		return reply, err
	}
	if management != nil {
		opts.memoryManagement = management
		opts.memoryAutoCapture = false
		opts.AllowStreaming = false
		opts.EnableSummary = false
		answer, runErr := al.runAgentLoop(ctx, agent, opts)
		if runErr != nil {
			if management.correction != nil {
				return "更正预览未生成，请重新发起更正。", nil
			}
			return management.receipt(), nil
		}
		return answer, nil
	}

	if pending := al.takePendingSkills(opts.Dispatch.SessionKey); len(pending) > 0 {
		opts.ForcedSkills = append(opts.ForcedSkills, pending...)
		logger.InfoCF("agent", "Applying pending skill override",
			map[string]any{
				"session_key": opts.Dispatch.SessionKey,
				"skills":      strings.Join(pending, ","),
			})
	}

	return al.runAgentLoop(ctx, agent, opts)
}

func (al *AgentLoop) resolveMessageRoute(msg bus.InboundMessage) (routing.ResolvedRoute, *AgentInstance, error) {
	registry := al.GetRegistry()
	inboundCtx := normalizedInboundContext(msg)
	route := registry.ResolveRoute(inboundCtx)

	agent, ok := registry.GetAgent(route.AgentID)
	if !ok {
		agent = registry.GetDefaultAgent()
	}
	if agent == nil {
		return routing.ResolvedRoute{}, nil, fmt.Errorf("no agent available for route (agent_id=%s)", route.AgentID)
	}

	return route, agent, nil
}

func isPrivateChat(msg bus.InboundMessage) bool {
	return msg.PrivateChat != nil && msg.Context.ChatType == "direct" && msg.Context.ChatID != ""
}

func inboundSessionKey(allocation session.Allocation, msg bus.InboundMessage) string {
	if isPrivateChat(msg) {
		return allocation.SessionKey
	}
	return resolveScopeKey(allocation.SessionKey, msg.SessionKey)
}

func (al *AgentLoop) allocateRouteSession(route routing.ResolvedRoute, msg bus.InboundMessage) session.Allocation {
	inbound := normalizedInboundContext(msg)
	allocation := session.AllocateRouteSession(session.AllocationInput{
		AgentID:       route.AgentID,
		Context:       inbound,
		SessionPolicy: route.SessionPolicy,
	})
	if isPrivateChat(msg) {
		// Even a shared route policy cannot mix private IM conversations.
		allocation.Scope.Dimensions = []string{"chat"}
		allocation.Scope.Values = map[string]string{
			"chat": fmt.Sprintf("private:%x", sha256.Sum256([]byte(inbound.ChatID))),
		}
		allocation.SessionKey = session.BuildSessionKey(allocation.Scope)
		allocation.SessionAliases = nil
	}
	return allocation
}

func (al *AgentLoop) processSystemMessage(
	ctx context.Context,
	msg bus.InboundMessage,
) (string, error) {
	if msg.Channel != "system" {
		return "", fmt.Errorf(
			"processSystemMessage called with non-system message channel: %s",
			msg.Channel,
		)
	}

	logger.InfoCF("agent", "Processing system message",
		map[string]any{
			"sender_id": msg.SenderID,
			"chat_id":   msg.ChatID,
		})

	// Parse origin channel from chat_id (format: "channel:chat_id")
	var originChannel, originChatID string
	if idx := strings.Index(msg.ChatID, ":"); idx > 0 {
		originChannel = msg.ChatID[:idx]
		originChatID = msg.ChatID[idx+1:]
	} else {
		originChannel = "cli"
		originChatID = msg.ChatID
	}

	// Extract subagent result from message content
	// Format: "Task 'label' completed.\n\nResult:\n<actual content>"
	content := msg.Content
	if idx := strings.Index(content, "Result:\n"); idx >= 0 {
		content = content[idx+8:] // Extract just the result part
	}

	// Skip internal channels - only log, don't send to user
	if constants.IsInternalChannel(originChannel) {
		logger.InfoCF("agent", "Subagent completed (internal channel)",
			map[string]any{
				"sender_id":   msg.SenderID,
				"content_len": len(content),
				"channel":     originChannel,
			})
		return "", nil
	}

	// Use default agent for system messages
	agent := al.GetRegistry().GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent for system message")
	}

	// Use the origin session for context
	sessionKey := session.BuildMainSessionKey(agent.ID)
	dispatch := DispatchRequest{
		SessionKey:  sessionKey,
		UserMessage: fmt.Sprintf("[System: %s] %s", msg.SenderID, msg.Content),
	}
	if originChannel != "" || originChatID != "" {
		dispatch.InboundContext = &bus.InboundContext{
			Channel:  originChannel,
			ChatID:   originChatID,
			ChatType: "direct",
			SenderID: msg.SenderID,
		}
	}

	return al.runAgentLoop(ctx, agent, processOptions{
		Dispatch:        dispatch,
		DefaultResponse: "Background task completed.",
		EnableSummary:   false,
		SendResponse:    true,
	})
}
