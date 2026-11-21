package agent

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/providers"
)

type memoryWriteProvider struct {
	t     *testing.T
	steps []func([]providers.Message, []providers.ToolDefinition) *providers.LLMResponse
}

func (*memoryWriteProvider) GetDefaultModel() string { return "test-model" }

func (p *memoryWriteProvider) Chat(
	_ context.Context, messages []providers.Message, defs []providers.ToolDefinition, _ string, _ map[string]any,
) (*providers.LLMResponse, error) {
	p.t.Helper()
	require.NotEmpty(p.t, p.steps, "unexpected LLM call")
	step := p.steps[0]
	p.steps = p.steps[1:]
	return step(messages, defs), nil
}

func memoryToolCall(name string, args map[string]any) *providers.LLMResponse {
	return &providers.LLMResponse{ToolCalls: []providers.ToolCall{{ID: name, Type: "function", Name: name, Arguments: args}}}
}

func (p *memoryWriteProvider) call(name string, args map[string]any) {
	p.steps = append(p.steps, func(_ []providers.Message, defs []providers.ToolDefinition) *providers.LLMResponse {
		require.True(p.t, memoryToolDefined(defs, name))
		return memoryToolCall(name, args)
	})
}

func memoryToolDefined(defs []providers.ToolDefinition, name string) bool {
	return slices.ContainsFunc(defs, func(d providers.ToolDefinition) bool { return d.Function.Name == name })
}

func (p *memoryWriteProvider) answer(text string) {
	p.steps = append(p.steps, func(_ []providers.Message, _ []providers.ToolDefinition) *providers.LLMResponse {
		return &providers.LLMResponse{Content: text}
	})
}

func memoryTestAgent(t *testing.T) (*AgentLoop, *memoryWriteProvider) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = t.TempDir()
	cfg.Agents.Defaults.ModelName = "test-model"
	p := &memoryWriteProvider{t: t}
	al := NewAgentLoop(cfg, bus.NewMessageBus(), p)
	t.Cleanup(al.Close)
	return al, p
}

func memoryWriteIM(t *testing.T, al *AgentLoop, channel, chat, kind, text, authored string) string {
	t.Helper()
	msg := bus.InboundMessage{
		Context: bus.InboundContext{Channel: channel, ChatID: chat, ChatType: kind, MessageID: "memory-test"},
		Content: text,
	}
	if authored != "" {
		msg.PrivateChat = &bus.PrivateChat{Text: authored}
	}
	out, err := al.processMessage(t.Context(), testInboundMessage(msg))
	require.NoError(t, err)
	return out
}

func TestMemoryWriteNaturalAcrossEntrances(t *testing.T) {
	al, p := memoryTestAgent(t)
	ctx := t.Context()
	db := al.GetRegistry().GetDefaultAgent().memoryDB
	p.call(memoryWriteName, map[string]any{"kind": "preference", "action": "upsert", "preferences": []any{
		map[string]any{"key": "language", "value": "中文"},
		map[string]any{"key": "style", "value": "简短一点"},
	}})
	request := "以后用中文、回答简短一点"
	require.Equal(t, "已保存偏好：language、style。", memoryWriteIM(t, al, "feishu", "owner", "direct", request, request))
	reply, err := al.ProcessLocalCLI(ctx, "帮我记一下，2026-05-21 原神打到第三章，任务还没交", "cli:test")
	require.NoError(t, err)
	require.Equal(t, "已记录：2026-05-21（#1）", reply)
	require.Empty(t, p.steps, "explicit saves do not make a model call")
	saved, ok, err := db.RecordByID(ctx, 1)
	require.NoError(t, err)
	require.True(t, ok)
	require.Contains(t, saved.Source, "cli session")
	p.call(memorySearchName, map[string]any{"query": "原神"})
	p.call(memoryWriteName, map[string]any{
		"kind": "record", "action": "upsert", "id": saved.ID, "old_text": "第三章", "text": "第四章",
	})
	request = "帮我记一下，原神进度改成第四章"
	preview := memoryWriteIM(t, al, "weixin", "owner", "direct", request, request)
	require.Contains(t, preview, saved.Body)
	require.Contains(t, preview, "原神打到第四章，任务还没交")
	prior, _, err := db.RecordByID(ctx, saved.ID)
	require.NoError(t, err)
	require.Equal(t, saved, prior)
	confirmation := "确认更正"
	require.Equal(t, "已更正记录 #1。", memoryWriteIM(t, al, "weixin", "owner", "direct", confirmation, confirmation))
	edited, ok, err := db.RecordByID(ctx, saved.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, saved.Source, edited.Source)
	require.Equal(t, saved.Date, edited.Date)
	require.Equal(t, saved.Category, edited.Category)
	require.Contains(t, edited.Body, "原神打到第四章，任务还没交")
	hits, err := db.Search(ctx, memory.Search{Query: "原神"})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	p.call(memorySearchName, map[string]any{"query": "原神"})
	p.call(memoryWriteName, map[string]any{"kind": "record", "action": "delete", "id": saved.ID})
	request = "删掉上次记的原神进度"
	require.Equal(t, "已删除记录 #1。", memoryWriteIM(t, al, "feishu", "owner", "direct", request, request))
	hits, err = db.Search(ctx, memory.Search{Query: "原神"})
	require.NoError(t, err)
	require.Empty(t, hits)
	p.steps = append(p.steps, func(msgs []providers.Message, _ []providers.ToolDefinition) *providers.LLMResponse {
		require.Contains(t, msgs[0].Content, "简短一点")
		require.Contains(t, msgs[0].Content, "中文")
		return memoryToolCall(memoryWriteName, map[string]any{"kind": "preference", "action": "delete", "key": "style"})
	})
	reply, err = al.ProcessLocalCLI(ctx, "删除简短回答的偏好", "cli:test")
	require.NoError(t, err)
	require.Equal(t, "已删除偏好：style。", reply)
	prefs, err := db.Preferences(ctx)
	require.NoError(t, err)
	require.Equal(t, []memory.Preference{{Key: "language", Value: "中文"}}, prefs)
}

func TestMemoryWriteAmbiguousChoiceAndStaleRecord(t *testing.T) {
	al, p := memoryTestAgent(t)
	ctx := t.Context()
	db := al.GetRegistry().GetDefaultAgent().memoryDB
	for _, day := range []string{"2026-11-20", "2026-11-21"} {
		_, err := db.Add(ctx, memory.Record{Body: "塞尔达宝箱清单", Date: day, Source: "cli"})
		require.NoError(t, err)
	}
	request := "删掉上次记的塞尔达宝箱清单"
	ask := func() string {
		p.call(memorySearchName, map[string]any{"query": "塞尔达"})
		p.call(memoryWriteName, map[string]any{"kind": "record", "action": "delete", "id": 1})
		p.answer("已删除")
		return memoryWriteIM(t, al, "weixin", "owner", "direct", request, request)
	}
	require.Contains(t, ask(), "你想删除哪一条")
	for _, id := range []int64{1, 2} {
		_, ok, err := db.RecordByID(ctx, id)
		require.NoError(t, err)
		require.True(t, ok)
	}
	for _, entry := range []struct{ channel, chat string }{{"feishu", "owner"}, {"weixin", "other"}} {
		p.steps = append(p.steps, func(_ []providers.Message, defs []providers.ToolDefinition) *providers.LLMResponse {
			require.False(t, memoryToolDefined(defs, memoryWriteName))
			return &providers.LLMResponse{Content: "没有可选记录"}
		})
		require.Equal(t, "没有可选记录", memoryWriteIM(t, al, entry.channel, entry.chat, "direct", "第二条", "第二条"))
	}
	changed, err := db.Update(ctx, 1, memory.Record{Body: "塞尔达宝箱清单（已编辑）"})
	require.NoError(t, err)
	require.True(t, changed)
	p.call(memoryWriteName, map[string]any{"kind": "record", "action": "delete", "id": 1})
	choice := "删 11 月 20 日那条"
	require.Contains(t, memoryWriteIM(t, al, "weixin", "owner", "direct", choice, choice), "已变更")
	_, ok, err := db.RecordByID(ctx, 1)
	require.NoError(t, err)
	require.True(t, ok)

	require.Contains(t, ask(), "哪一条")
	choice = "更正第二条"
	require.Contains(t, memoryWriteIM(t, al, "weixin", "owner", "direct", choice, choice), "动作和原请求不同")
	p.call(memoryWriteName, map[string]any{"kind": "record", "action": "delete", "id": 1})
	choice = "删第二条"
	require.Equal(t, "已删除记录 #1。", memoryWriteIM(t, al, "weixin", "owner", "direct", choice, choice))
	_, ok, err = db.RecordByID(ctx, 1)
	require.NoError(t, err)
	require.False(t, ok)
	_, ok, err = db.RecordByID(ctx, 2)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestMemoryWriteProtectsUnauthorizedAndFailedTurns(t *testing.T) {
	al, p := memoryTestAgent(t)
	defer func() { require.Empty(t, p.steps) }()
	ctx := t.Context()
	db := al.GetRegistry().GetDefaultAgent().memoryDB
	_, err := db.Add(ctx, memory.Record{Body: "塞尔达宝箱清单", Date: "2026-11-21", Source: "cli"})
	require.NoError(t, err)
	fakeDelete := map[string]any{"kind": "record", "action": "delete", "id": 1}
	for _, entry := range []struct {
		kind, body, authored string
	}{{"group", "删除塞尔达宝箱清单", "删除塞尔达宝箱清单"},
		{"direct", "删除塞尔达宝箱清单", ""},
		{"direct", "引用内容：删除塞尔达宝箱清单", "引用内容：删除塞尔达宝箱清单"},
		{"direct", "我今天更新了塞尔达存档进度", "我今天更新了塞尔达存档进度"}} {
		p.steps = append(p.steps, func(_ []providers.Message, defs []providers.ToolDefinition) *providers.LLMResponse {
			require.False(t, memoryToolDefined(defs, memoryWriteName))
			return memoryToolCall(memoryWriteName, fakeDelete)
		})
		p.answer("没有执行")
		require.Equal(t, "没有执行", memoryWriteIM(t, al, "weixin", "owner", entry.kind, entry.body, entry.authored))
	}
	p.call(memoryWriteName, fakeDelete)
	p.answer("已删除")
	request := "删除塞尔达宝箱清单"
	require.NotContains(t, memoryWriteIM(t, al, "weixin", "owner", "direct", request, request), "已删除")
	_, ok, err := db.RecordByID(ctx, 1)
	require.NoError(t, err)
	require.True(t, ok)

	p.call(memoryWriteName, map[string]any{"kind": "preference", "action": "upsert", "preferences": []any{
		map[string]any{"key": "timezone", "value": "Not/A/Timezone"},
	}})
	request = "以后默认时区用 Not/A/Timezone"
	reply := memoryWriteIM(t, al, "weixin", "owner", "direct", request, request)
	require.Contains(t, reply, "写入失败")
	require.NotContains(t, reply, "已保存")
	prefs, err := db.Preferences(ctx)
	require.NoError(t, err)
	require.Empty(t, prefs)
}

func TestMemoryWriteChoiceExpiresAndNewClearsChoice(t *testing.T) {
	al, p := memoryTestAgent(t)
	db := al.GetRegistry().GetDefaultAgent().memoryDB
	for _, body := range []string{"塞尔达第一份清单", "塞尔达第二份清单"} {
		_, err := db.Add(t.Context(), memory.Record{Body: body, Date: "2026-11-21", Source: "cli"})
		require.NoError(t, err)
	}
	ask := func() {
		p.call(memorySearchName, map[string]any{"query": "塞尔达"})
		p.answer("要删哪条？")
		req := "删掉塞尔达清单"
		require.Contains(t, memoryWriteIM(t, al, "weixin", "owner", "direct", req, req), "哪一条")
	}
	ask()
	al.pendingMemoryChoices.Range(func(_, value any) bool {
		value.(*pendingMemoryChoice).expires = time.Now().Add(-time.Second)
		return false
	})
	require.Contains(t, memoryWriteIM(t, al, "weixin", "owner", "direct", "第二条", "第二条"), "过期")
	ask()
	require.NotEmpty(t, memoryWriteIM(t, al, "weixin", "owner", "direct", "/new", "/new"))
	p.steps = append(p.steps, func(_ []providers.Message, defs []providers.ToolDefinition) *providers.LLMResponse {
		require.False(t, memoryToolDefined(defs, memoryWriteName))
		return &providers.LLMResponse{Content: "重新查找"}
	})
	require.Equal(t, "重新查找", memoryWriteIM(t, al, "weixin", "owner", "direct", "第二条", "第二条"))
	for _, id := range []int64{1, 2} {
		_, ok, err := db.RecordByID(t.Context(), id)
		require.NoError(t, err)
		require.True(t, ok)
	}
}

func TestMemoryIntentDistinguishesManagementFromConversation(t *testing.T) {
	for _, text := range []string{
		"分析一下「以后用中文」", "我今天更新了原神进度", "不要删除原神记录",
		"怎么删除原神记录", "删除目录 /tmp/example", "用中文解释这句话",
		"「删除原神进度」", "我们昨天说过要删掉原神进度", "帮我记一下原神第三章",
		"我以后想学中文，有什么教材推荐？", "以后想学中文，推荐点教材", "以后用中文读原著",
		"以后用中文回答，顺便推荐教材？", "以后用中文，推荐本教材可以吗？", "以后时区用什么",
		"以后用中文回答，推荐一本中文教材。", "以后回答简短一点并推荐一本中文教材。",
		"From now on use Chinese and recommend a textbook.", "以后用中文回答，推荐本教材，好吗？",
		"我在考虑以后设置中文回答偏好", "设置偏好用中文，推荐一本教材。",
		"不要把记录 #1 里的第三章改成第五章", "请先别把记录 #1 里的第三章改成第五章",
		"如果把记录 #1 里的第三章改成第五章会怎样", "把记录 #1 里的第三章改成第五章，推荐一本教材。",
		"Going forward I want to learn Chinese.", "From now on use Chinese textbooks.",
		"保存文件并退出", "删除中文教材", "record a turn",
	} {
		require.Empty(t, detectMemoryIntent(text).kind, strings.TrimSpace(text))
	}
	for _, text := range []string{
		"以后用中文、回答简短一点", "以后回答简短一点", "请你以后都用中文回答。",
		"以后默认时区用 Asia/Shanghai", "以后叫我小明", "以后用公制单位",
		"以后回复用中文可以吗？", "以后回答简短一点，好吗？", "以后用 Markdown",
		"From now on use Chinese and keep replies brief.", "Going forward, keep replies brief.",
		"请设置偏好用中文", "请把时区改成 Asia/Shanghai",
	} {
		require.Equal(t, memoryIntent{kind: "preference", action: "upsert"}, detectMemoryIntent(text), text)
	}
}

func TestOrdinaryLanguageQuestionKeepsAnswer(t *testing.T) {
	al, p := memoryTestAgent(t)
	request := "我以后想学中文，有什么教材推荐？"
	answer := "可以从《HSK 标准教程》开始。"
	p.steps = append(p.steps, func(_ []providers.Message, defs []providers.ToolDefinition) *providers.LLMResponse {
		if memoryToolDefined(defs, memoryWriteName) {
			t.Error("教材推荐不应获得记忆写入工具")
		}
		return &providers.LLMResponse{Content: answer}
	})
	require.Equal(t, answer, memoryWriteIM(t, al, "feishu", "owner", "direct", request, request))
	db := al.GetRegistry().GetDefaultAgent().memoryDB
	prefs, err := db.Preferences(t.Context())
	require.NoError(t, err)
	require.Empty(t, prefs)
}

func TestMemoryWritePartialSearchDoesNotAuthorizeUnrelatedRecord(t *testing.T) {
	al, p := memoryTestAgent(t)
	db := al.GetRegistry().GetDefaultAgent().memoryDB
	id, err := db.Add(t.Context(), memory.Record{Body: "原神还没拿的宝箱清单", Date: "2026-11-21", Source: "cli"})
	require.NoError(t, err)
	request := "删掉上次记的塞尔达宝箱清单"
	p.call(memorySearchName, map[string]any{"query": "宝箱"})
	p.call(memoryWriteName, map[string]any{"kind": "record", "action": "delete", "id": id})
	p.answer("已删除")
	reply := memoryWriteIM(t, al, "weixin", "owner", "direct", request, request)
	require.Contains(t, reply, "请回复")
	require.NotContains(t, reply, "已删除")
	_, ok, err := db.RecordByID(t.Context(), id)
	require.NoError(t, err)
	require.True(t, ok)

	id, err = db.Add(t.Context(), memory.Record{Body: "塞尔达的宝箱清单", Date: "2026-11-21", Source: "cli"})
	require.NoError(t, err)
	p.call(memorySearchName, map[string]any{"query": "塞尔达"})
	p.call(memoryWriteName, map[string]any{"kind": "record", "action": "delete", "id": id})
	require.Equal(t, "已删除记录 #2。", memoryWriteIM(t, al, "weixin", "owner", "direct", request, request))
}

func TestMemoryWriteWithoutToolResultDoesNotConfirmModelClaim(t *testing.T) {
	al, p := memoryTestAgent(t)
	p.answer("已保存")
	request := "以后回答简短一点"
	require.NotContains(t, memoryWriteIM(t, al, "feishu", "owner", "direct", request, request), "已保存")
	db := al.GetRegistry().GetDefaultAgent().memoryDB
	keys, err := db.DueReviews(t.Context(), time.Now(), time.Now())
	require.NoError(t, err)
	require.Empty(t, keys, "explicit management requests must never enter scheduled review")
}

func TestMemoryWriteCorrectionChecksExplicitOldAndNewValues(t *testing.T) {
	al, p := memoryTestAgent(t)
	ctx := t.Context()
	db := al.GetRegistry().GetDefaultAgent().memoryDB
	run := func(text string) string {
		t.Helper()
		reply, err := al.ProcessLocalCLI(ctx, text, "cli:correction")
		require.NoError(t, err)
		return reply
	}
	const original = "原神主线第三章，支线第四章"
	require.Contains(t, run("帮我记一下，"+original), "（#1）")
	request := "把记录 #1 里的第三章改成第五章"
	for _, change := range []struct {
		old, new string
		message  string
	}{
		{"第四章", "第五章", "旧值与原话不符"},
		{"第三章", "第三章", "完整替换内容"},
	} {
		p.call(memorySearchName, map[string]any{"id": 1})
		p.call(memoryWriteName, map[string]any{
			"kind": "record", "action": "upsert", "id": 1, "old_text": change.old, "text": change.new,
		})
		require.Contains(t, run(request), change.message)
		r, ok, err := db.RecordByID(ctx, 1)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, original, r.Body)
	}
	p.call(memorySearchName, map[string]any{"id": 1})
	p.call(memoryWriteName, map[string]any{
		"kind": "record", "action": "upsert", "id": 1, "old_text": "第三章", "text": "第五章",
	})
	require.Equal(t, "已更正记录 #1。", run(request))
	r, ok, err := db.RecordByID(ctx, 1)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "原神主线第五章，支线第四章", r.Body)
}

func TestMemoryWriteCorrectionPreviewIsScopedAndVersioned(t *testing.T) {
	al, p := memoryTestAgent(t)
	ctx := t.Context()
	db := al.GetRegistry().GetDefaultAgent().memoryDB
	const original = "原神主线第三章，支线第三章"
	id, err := db.Add(ctx, memory.Record{Body: original, Date: "2026-11-24", Source: "cli"})
	require.NoError(t, err)
	request := "把记录 #1 里的「第三章」改成「第五章」"
	ask := func() string {
		p.call(memorySearchName, map[string]any{"id": id})
		p.call(memoryWriteName, map[string]any{
			"kind": "record", "action": "upsert", "id": id, "old_text": "主线第三章", "text": "第五章",
		})
		return memoryWriteIM(t, al, "weixin", "owner", "direct", request, request)
	}
	preview := ask()
	require.Contains(t, preview, original)
	require.Contains(t, preview, "原神主线第五章，支线第三章")
	r, _, err := db.RecordByID(ctx, id)
	require.NoError(t, err)
	require.Equal(t, original, r.Body)

	confirm := "确认更正"
	require.Equal(t, "当前会话没有待确认的更正。", memoryWriteIM(t, al, "feishu", "owner", "direct", confirm, confirm))
	updated, err := db.Update(ctx, id, memory.Record{Body: original + "，补充说明"})
	require.NoError(t, err)
	require.True(t, updated)
	require.Contains(t, memoryWriteIM(t, al, "weixin", "owner", "direct", confirm, confirm), "已变更")
	r, _, err = db.RecordByID(ctx, id)
	require.NoError(t, err)
	require.Equal(t, original+"，补充说明", r.Body)

	require.Contains(t, ask(), "请核对")
	cancel := "取消更正"
	require.Equal(t, "已取消更正。", memoryWriteIM(t, al, "weixin", "owner", "direct", cancel, cancel))
	require.Contains(t, ask(), "请核对")
	require.NotEmpty(t, memoryWriteIM(t, al, "weixin", "owner", "direct", "/new", "/new"))
	require.Equal(t, "当前会话没有待确认的更正。", memoryWriteIM(t, al, "weixin", "owner", "direct", confirm, confirm))
	require.Contains(t, ask(), "请核对")
	require.Equal(t, "已更正记录 #1。", memoryWriteIM(t, al, "weixin", "owner", "direct", confirm, confirm))
	r, _, err = db.RecordByID(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "原神主线第五章，支线第三章，补充说明", r.Body)
}

func TestMemoryWriteNegationAndMixedRequestsStayInConversation(t *testing.T) {
	al, p := memoryTestAgent(t)
	ctx := t.Context()
	db := al.GetRegistry().GetDefaultAgent().memoryDB
	const original = "原神主线第三章，支线第四章"
	run := func(text string) string {
		t.Helper()
		reply, err := al.ProcessLocalCLI(ctx, text, "cli:review")
		require.NoError(t, err)
		return reply
	}
	require.Contains(t, run("帮我记一下，"+original), "（#1）")

	const answer = "可以从《HSK 标准教程》开始。"
	p.steps = append(p.steps, func(_ []providers.Message, defs []providers.ToolDefinition) *providers.LLMResponse {
		require.False(t, memoryToolDefined(defs, memoryWriteName))
		return &providers.LLMResponse{Content: answer}
	})
	require.Equal(t, answer, run("以后用中文回答，顺便推荐一本中文教材。"))

	p.steps = append(p.steps, func(_ []providers.Message, defs []providers.ToolDefinition) *providers.LLMResponse {
		if memoryToolDefined(defs, memoryWriteName) {
			return memoryToolCall(memorySearchName, map[string]any{"id": 1})
		}
		// Simulate a provider proposing a write even though the tool was omitted.
		return memoryToolCall(memoryWriteName, map[string]any{
			"kind": "record", "action": "upsert", "id": 1, "old_text": "第三章", "text": "第五章",
		})
	}, func(_ []providers.Message, defs []providers.ToolDefinition) *providers.LLMResponse {
		if memoryToolDefined(defs, memoryWriteName) {
			return memoryToolCall(memoryWriteName, map[string]any{
				"kind": "record", "action": "upsert", "id": 1, "old_text": "第三章", "text": "第五章",
			})
		}
		return &providers.LLMResponse{Content: "好的。"}
	})
	reply := run("不要把记录 #1 里的第三章改成第五章")
	require.Equal(t, "好的。", reply)
	r, ok, err := db.RecordByID(ctx, 1)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, original, r.Body)
	prefs, err := db.Preferences(ctx)
	require.NoError(t, err)
	require.Empty(t, prefs)
	keys, err := db.DueReviews(ctx, time.Now(), time.Now())
	require.NoError(t, err)
	require.Len(t, keys, 1)
	batch, err := db.ReadReview(ctx, keys[0])
	require.NoError(t, err)
	require.Len(t, batch.Notes, 1)
	require.Contains(t, batch.Notes[0].Text, "推荐一本中文教材")
}

func TestDirectMemoryCorrectionRequiresWholeAffirmativeRequest(t *testing.T) {
	for _, text := range []string{
		"不要把记录 #1 里的第三章改成第五章",
		"我在想把记录 #1 里的第三章改成第五章",
		"把记录 #1 里的第三章改成第五章，推荐教材",
		"如果把记录 #1 里的第三章改成第五章",
		"把记录 #2 里的第三章改成第五章",
	} {
		require.False(t, matchesDirectCorrection(text, 1, "第三章", "第五章"), text)
	}
	require.True(t, matchesDirectCorrection("请把记录 #1 里的「第三章」改成「第五章」。", 1, "第三章", "第五章"))
	require.True(t, matchesDirectCorrection("请把记录 #1 里的「不要忘带伞」改成「带伞」。", 1, "不要忘带伞", "带伞"))
}
