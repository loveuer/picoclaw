package agent

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/providers"
)

type memoryAcceptanceProvider struct {
	t        *testing.T
	source   string
	calls    [][]providers.Message
	toolDefs [][]providers.ToolDefinition
}

func (p *memoryAcceptanceProvider) GetDefaultModel() string { return "test-model" }

func (p *memoryAcceptanceProvider) Chat(
	_ context.Context,
	msgs []providers.Message,
	defs []providers.ToolDefinition,
	_ string,
	_ map[string]any,
) (*providers.LLMResponse, error) {
	p.calls = append(p.calls, append([]providers.Message(nil), msgs...))
	p.toolDefs = append(p.toolDefs, defs)
	last := msgs[len(msgs)-1]
	if last.Role == "tool" {
		require.Contains(p.t, last.Content, "2026-05-21")
		source := p.source
		if source == "" {
			source = "game.md:1-1"
		}
		require.Contains(p.t, last.Content, source)
		require.Contains(p.t, last.Content, "第三章")
		return &providers.LLMResponse{Content: "2026-05-21 玩到第三章（" + source + "）。"}, nil
	}
	if strings.Contains(last.Content, "上次玩到哪") {
		require.Contains(p.t, msgs[0].Content, "中文简洁")
		require.Contains(p.t, msgs[0].Content, "call memory_search")
		require.NotContains(p.t, msgs[0].Content, "第三章")
		found := false
		for _, d := range defs {
			found = found || d.Function.Name == memorySearchName
		}
		require.True(p.t, found)
		return &providers.LLMResponse{
			ToolCalls: []providers.ToolCall{
				{ID: "lookup", Type: "function", Name: memorySearchName, Arguments: map[string]any{"query": "原神"}},
			},
		}, nil
	}
	return &providers.LLMResponse{Content: "收到"}, nil
}

func TestPrivateMemoryAcceptanceAcrossRestart(t *testing.T) {
	root := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = filepath.Join(root, "workspace")
	cfg.Agents.Defaults.ModelName = "test-model"
	p := &memoryAcceptanceProvider{t: t}
	open := func() *AgentLoop { return NewAgentLoop(cfg, bus.NewMessageBus(), p) }
	al := open()
	m := newConversationTestMessage("", "100")
	m.Channel, m.Context.Channel = "weixin", "weixin"
	m.Context.MessageID = "42"
	m.PrivateChat = &bus.PrivateChat{}
	process := func(text string) (string, error) {
		m.Content = text
		m.PrivateChat.Text = text
		return al.processMessage(t.Context(), m)
	}
	_, err := al.GetRegistry().GetDefaultAgent().memoryDB.Add(t.Context(), memory.Record{
		Body: "原神，玩到第三章。", Date: "2026-05-21", Source: "game.md:1-1",
	})
	require.NoError(t, err)
	_, err = process("/preference set style 中文简洁")
	require.NoError(t, err)
	_, err = process("旧的上下文")
	require.NoError(t, err)
	_, err = process("再聊一轮")
	require.NoError(t, err)
	_, err = process("/new")
	require.NoError(t, err)
	al.Close()
	al = open()
	defer al.Close()
	result, err := process("我原神上次玩到哪")
	require.NoError(t, err)
	require.Contains(t, result, "2026-05-21")
	require.Contains(t, result, "第三章")
	for _, msg := range p.calls[len(p.calls)-2] {
		require.NotContains(t, msg.Content, "旧的上下文")
	}
	route, a, err := al.resolveMessageRoute(m)
	require.NoError(t, err)
	key := al.allocateRouteSession(route, m).SessionKey
	for _, msg := range a.Sessions.GetHistory(key) {
		if msg.Role == "tool" {
			require.NotContains(t, msg.Content, "game.md")
			require.Contains(t, msg.Content, "omitted")
		}
	}
	private := m.PrivateChat
	m.PrivateChat, m.SessionKey = nil, key
	m.Content = "cannot reuse the private session"
	_, err = al.processMessage(t.Context(), m)
	require.ErrorContains(t, err, "cannot be accessed")
	m.PrivateChat, m.SessionKey = private, ""
	m.Context.ChatType = "group"
	m.Context.ChatID = "200"
	_, err = process("群聊")
	require.NoError(t, err)
	last := p.calls[len(p.calls)-1]
	for _, msg := range last {
		require.NotContains(t, msg.Content, "中文简洁")
		require.NotContains(t, msg.Content, "第三章")
	}
	for _, d := range p.toolDefs[len(p.toolDefs)-1] {
		require.NotEqual(t, memorySearchName, d.Function.Name)
	}
	m.Context.ChatType, m.Context.ChatID = "direct", "100"
	result, err = process("记住我今天完成了第四章")
	require.NoError(t, err)
	require.Contains(t, result, "已记录")
	m.PrivateChat.Text = ""
	m.Content = "/memory add should not be accepted"
	result, err = al.processMessage(t.Context(), m)
	require.NoError(t, err)
	require.NotContains(t, result, "已记录")
}

func TestPrivateMemoryDoesNotReadLegacyFilesOrExposeProtectedPaths(t *testing.T) {
	workspace := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(workspace, "memory"), 0o700))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(workspace, "memory", "MEMORY.md"), []byte("secret-from-old-file"), 0o600),
	)
	require.NotContains(t, NewContextBuilder(workspace).BuildSystemPromptWithCache(), "secret-from-old-file")
	require.Error(t, workspaceToolPath(workspace, "memory/MEMORY.md"))
	require.Error(t, workspaceToolPath(workspace, memory.Path(config.GetHome())))
	require.NoError(
		t,
		os.Symlink(filepath.Join(workspace, "memory", "MEMORY.md"), filepath.Join(workspace, "innocent.md")),
	)
	require.Error(t, workspaceToolPath(workspace, "innocent.md"))
	require.NoError(t, workspaceToolPath(workspace, "ordinary.md"))
	require.True(t, (&memorySearchTool{}).Execute(t.Context(), map[string]any{"query": "原神"}).IsError)
}

func TestNaturalMemoryRequestRecognizesColloquialChinese(t *testing.T) {
	for text, body := range map[string]string{
		"帮我记一下 xxx": "xxx", "记录一下 xxx": "xxx", "请帮我记一下：原神第三章": "原神第三章",
		"请记录一下，今天读到第五章": "今天读到第五章", "记住我今天完成了第四章": "我今天完成了第四章",
	} {
		k, arg, ok := memoryCommand(text)
		require.True(t, ok, text)
		require.Equal(t, "/memory", k)
		require.Equal(t, "add "+body, arg)
	}
	for _, text := range []string{"你能帮我记一下吗？", "记住原神上次玩到哪里了？", "记一下 > 引用"} {
		_, _, ok := memoryCommand(text)
		require.False(t, ok, text)
	}
}

func TestThreeEntrancesShareMemoryWithoutSharingHistory(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = t.TempDir()
	cfg.Agents.Defaults.ModelName = "test-model"
	var calls [][]providers.Message
	p := &capturingMockProvider{response: "收到", captureFn: func(messages []providers.Message) {
		calls = append(calls, append([]providers.Message(nil), messages...))
	}}
	al := NewAgentLoop(cfg, bus.NewMessageBus(), p)
	defer al.Close()
	local := func(input string) string {
		t.Helper()
		reply, err := al.ProcessLocalCLI(t.Context(), input, "cli:default")
		require.NoError(t, err)
		return reply
	}
	message := func(channel, typ, text string, admitted bool) bus.InboundMessage {
		msg := bus.InboundMessage{
			Context: bus.InboundContext{Channel: channel, ChatID: "same-chat-id", ChatType: typ, MessageID: "m-1"},
			Content: text,
		}
		if admitted {
			msg.PrivateChat = &bus.PrivateChat{Text: text}
		}
		return testInboundMessage(msg)
	}
	im := func(channel, typ, text string, admitted bool) string {
		t.Helper()
		reply, err := al.processMessage(t.Context(), message(channel, typ, text, admitted))
		require.NoError(t, err)
		return reply
	}
	hasText := func(msgs []providers.Message, content string) bool {
		return slices.ContainsFunc(msgs, func(m providers.Message) bool { return strings.Contains(m.Content, content) })
	}
	require.Contains(t, local("帮我记一下，2026-05-21 原神打到第三章"), "已记录：2026-05-21（#1）")
	local("独立终端聊天")
	for _, channel := range []string{"feishu", "weixin"} {
		export := im(channel, "direct", "/memory export", true)
		require.Contains(t, export, "原神打到第三章")
		require.Contains(t, export, "2026-05-21")
		require.Contains(t, export, "cli session")
	}
	im("feishu", "direct", "独立飞书聊天", true)
	im("weixin", "direct", "独立微信聊天", true)
	require.False(t, hasText(calls[len(calls)-1], "独立飞书聊天"))
	require.False(t, hasText(calls[len(calls)-1], "独立终端聊天"))

	db := al.GetRegistry().GetDefaultAgent().memoryDB
	m := message("weixin", "direct", "", true)
	route, _, err := al.resolveMessageRoute(m)
	require.NoError(t, err)
	wKey := al.allocateRouteSession(route, m).SessionKey
	before, err := db.ReadReview(t.Context(), wKey)
	require.NoError(t, err)
	require.NotEmpty(t, before.Notes)
	require.NoError(t, db.SetSummary(t.Context(), wKey, "微信旧摘要暗号"))

	require.Contains(t, im("weixin", "direct", "/preference set style 中文简洁", true), "已保存")
	im("weixin", "direct", "/new 新问题", true)
	require.True(t, hasText(calls[len(calls)-1], "中文简洁"))
	require.False(t, hasText(calls[len(calls)-1], "独立微信聊天"))
	require.False(t, hasText(calls[len(calls)-1], "微信旧摘要暗号"))
	after, err := db.ReadReview(t.Context(), wKey)
	require.NoError(t, err)
	require.Len(t, after.Notes, 1)
	require.Equal(t, "新问题", after.Notes[0].Text)
	require.Contains(t, im("weixin", "direct", "/memory export", true), "原神打到第三章")
	local("检查回答风格")
	require.True(t, hasText(calls[len(calls)-1], "中文简洁"))
	require.True(t, hasText(calls[len(calls)-1], "独立终端聊天"))
	im("feishu", "direct", "继续聊天", true)
	require.True(t, hasText(calls[len(calls)-1], "独立飞书聊天"))
	require.Contains(t, im("weixin", "direct", "/memory update 1 原神已打到第四章", true), "已更正")
	require.Contains(t, local("/memory export"), "原神已打到第四章")
	require.Contains(t, local("/memory delete 1"), "已删除")
	require.Contains(t, im("feishu", "direct", "/memory export", true), "没有更多记录")

	for _, test := range []struct {
		kind     string
		admitted bool
	}{{"group", true}, {"direct", false}} {
		require.Contains(t, im("weixin", test.kind, "/memory export", test.admitted), "没有保存")
		require.Contains(t, im("weixin", test.kind, "记录一下，不能写入", test.admitted), "没有保存")
		im("weixin", test.kind, "普通消息", test.admitted)
		require.False(t, hasText(calls[len(calls)-1], "中文简洁"))
	}
	require.Contains(t, local("/memory export"), "没有更多记录")
}

type chestRecallProvider struct {
	t *testing.T
}

func (*chestRecallProvider) GetDefaultModel() string { return "fixture" }

func (p *chestRecallProvider) Chat(
	_ context.Context, msgs []providers.Message, defs []providers.ToolDefinition, _ string, _ map[string]any,
) (*providers.LLMResponse, error) {
	if msgs[0].Content == memoryClassificationPrompt {
		return &providers.LLMResponse{Content: "null"}, nil
	}
	last := msgs[len(msgs)-1]
	if last.Role == "tool" {
		for _, evidence := range []string{"协莫拉塔神庙", "吉兹卡夫伊神庙", "塔罗尼席神庙", "2026-11-21", "cli session"} {
			require.Contains(p.t, last.Content, evidence)
		}
		return &providers.LLMResponse{Content: "2026-11-21 的 CLI 记录：协莫拉塔神庙、吉兹卡夫伊神庙、塔罗尼席神庙。"}, nil
	}
	require.Contains(p.t, msgs[0].Content, "retry with the subject alone")
	require.True(p.t, slices.ContainsFunc(defs, func(d providers.ToolDefinition) bool {
		return d.Function.Name == memorySearchName
	}))
	require.Contains(p.t, last.Content, "帮我查查记录 我还有哪些宝箱没拿")
	return &providers.LLMResponse{ToolCalls: []providers.ToolCall{{
		ID: "chest-recall", Type: "function", Name: memorySearchName,
		Arguments: map[string]any{"query": "宝箱", "alternatives": []any{"箱子", "神庙", "储藏箱", "BOTW"}, "category": "日常/游戏"},
	}}}, nil
}

func TestWeixinRecallsCLIChestRecordOnFirstSearch(t *testing.T) {
	for _, ending := range []string{"的箱子都还没拿。", "都还没拿奖励。"} {
		t.Run(ending, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Agents.Defaults.Workspace = t.TempDir()
			cfg.Agents.Defaults.ModelName = "fixture"
			al := NewAgentLoop(cfg, bus.NewMessageBus(), &chestRecallProvider{t: t})
			defer al.Close()
			// Put the important facts near the end so an excerpt must find a
			// matching keyword, not simply return the record's opening characters.
			note := "帮我记一下，2026-11-21 " + strings.Repeat("这是一段背景描述。", 70) +
				"协莫拉塔神庙、吉兹卡夫伊神庙、塔罗尼席神庙" + ending
			reply, err := al.ProcessLocalCLI(t.Context(), note, "cli:default")
			require.NoError(t, err)
			require.Contains(t, reply, "已记录：2026-11-21")
			query := "帮我查查记录 我还有哪些宝箱没拿"
			reply, err = al.processMessage(t.Context(), bus.InboundMessage{
				Context:     bus.InboundContext{Channel: "weixin", ChatID: "alice", ChatType: "direct", MessageID: "query"},
				PrivateChat: &bus.PrivateChat{Text: query},
				Content:     query,
			})
			require.NoError(t, err)
			require.Contains(t, reply, "塔罗尼席神庙")

		})
	}
}

func TestMemoryCategoryRequiresUserScope(t *testing.T) {
	require.Empty(t, explicitMemoryCategory("查查记录，旷野之息还有哪些宝箱没拿", "日常/游戏"))
	require.Empty(t, explicitMemoryCategory("只查日常/游戏", "日常"))
	require.Equal(t, "日常/游戏", explicitMemoryCategory("只在「日常/游戏」分类查宝箱", "日常/游戏"))
	require.Equal(t, "日常", explicitMemoryCategory("在日常分类查宝箱", "日常"))
}

func TestMemoryStartupIgnoresOldWorkspaceHistory(t *testing.T) {
	root := t.TempDir()
	t.Setenv(config.EnvHome, root)
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = filepath.Join(root, "workspace")
	legacy := filepath.Join(cfg.Agents.Defaults.Workspace, "sessions", "old.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacy), 0o700))
	old := []byte("{\"role\":\"user\",\"content\":\"old session\"}\n")
	require.NoError(t, os.WriteFile(legacy, old, 0o600))

	release, err := PrepareMemory(cfg)
	require.NoError(t, err)
	defer release()
	db, err := memory.Open(memory.Path(root))
	require.NoError(t, err)
	defer db.Close()
	require.Empty(t, db.ListSessions())
	contents, err := os.ReadFile(legacy)
	require.NoError(t, err)
	require.Equal(t, old, contents)
}
