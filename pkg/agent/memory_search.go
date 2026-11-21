package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// Only the current user's own text can authorize narrowing search by category.
func explicitMemoryCategory(authored, category string) string {
	if category == "" {
		return ""
	}
	text := strings.ToLower(authored)
	text = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || strings.ContainsRune("「」『』“”\"':：", r) {
			return -1
		}
		return r
	}, text)
	path := strings.ToLower(strings.ReplaceAll(category, " ", ""))
	for _, suffix := range []string{"分类", "类别"} {
		if strings.Contains(text, path+suffix) {
			return category
		}
	}
	for _, prefix := range []string{"分类", "类别", "只查", "仅查", "只在", "仅在", "category"} {
		if _, after, found := strings.Cut(text, prefix+path); found && !strings.HasPrefix(after, "/") {
			return category
		}
	}
	return ""
}

const memorySearchName = "memory_search"

type memorySearchTool struct{ db *memory.SQLite }

func (*memorySearchTool) Name() string { return memorySearchName }
func (*memorySearchTool) Description() string {
	return "Search this instance's saved records by keywords, category, alias, tag or dates. Use a short main keyword and supply independent alternate spellings, synonyms or related terms in alternatives. Only use relevant results; cite date and source."
}

func (*memorySearchTool) Parameters() map[string]any {
	fields := map[string]any{}
	for _, k := range []string{"query", "alias", "tag", "category", "from", "to"} {
		desc := k
		switch k {
		case "query":
			desc = "One or two short keywords, main subject first. Use 宝箱, not 宝箱 没拿 游戏 收集 进度. Separate keywords by spaces."
		case "alias", "tag":
			desc = "Exact filter. For alternate spellings, synonyms, abbreviations or related tags, use alternatives instead."
		case "category":
			desc = "Optional category path, e.g. 日常/游戏. A parent path includes its descendants. Only set when the user explicitly restricts the scope to that category."
		case "from", "to":
			desc += " (YYYY-MM-DD, inclusive)"
		}
		fields[k] = map[string]any{"type": "string", "description": desc}
	}
	fields["id"] = map[string]any{"type": "integer", "minimum": 1, "description": "Exact record ID, only when explicitly supplied by the user."}
	fields["alternatives"] = map[string]any{
		"type":        "array",
		"items":       map[string]any{"type": "string"},
		"maxItems":    memory.MaxSearchAlternatives,
		"description": "Independent short queries, such as 箱子, 神庙宝箱, BOTW. These are OR alternatives, not extra required words.",
	}
	return map[string]any{"type": "object", "properties": fields, "additionalProperties": false}
}

func (t *memorySearchTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	access, _ := ctx.Value(privateMemoryKey{}).(privateMemoryAccess)
	if !access.allowed || t.db == nil {
		return tools.ErrorResult("Private memory is unavailable in this conversation.")
	}
	get := func(k string) string { v, _ := args[k].(string); return strings.TrimSpace(v) }
	for k := range args {
		if !slices.Contains([]string{"query", "alternatives", "alias", "tag", "category", "from", "to", "id"}, k) {
			return tools.ErrorResult("Unsupported memory search parameter.")
		}
	}
	q := memory.Search{
		Query:    get("query"),
		Alias:    get("alias"),
		Tag:      get("tag"),
		Category: explicitMemoryCategory(access.authored, get("category")),
		From:     get("from"),
		To:       get("to"),
	}
	if raw, ok := args["alternatives"]; ok {
		switch values := raw.(type) {
		case []string:
			q.Alternatives = values
		case []any:
			for _, value := range values {
				word, valid := value.(string)
				if !valid {
					return tools.ErrorResult("Each memory search alternative must be text.")
				}
				q.Alternatives = append(q.Alternatives, word)
			}
		default:
			return tools.ErrorResult("Memory search alternatives must be an array of short queries.")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	hits, e := t.search(ctx, access, q, args["id"])
	if e != nil {
		if access.management != nil {
			access.management.searchFailed = true
		}
		return tools.ErrorResult(
			"Memory search failed. Check the search parameters or try again. Do not report this as no matches.",
		)
	}
	if access.management != nil {
		access.management.rememberHits(hits)
	}
	if len(hits) == 0 {
		return tools.SilentResult(
			"No saved records matched this query. If still relevant to the request, retry with the main keyword alone or a known alias before concluding there is no record.",
		)
	}
	type hit struct {
		ID       int64    `json:"id"`
		Text     string   `json:"original_text"`
		Date     string   `json:"date"`
		Source   string   `json:"source"`
		Tags     []string `json:"tags,omitempty"`
		Category string   `json:"category,omitempty"`
	}
	out := make([]hit, 0, len(hits))
	keywords := strings.Join(q.Queries(), " ")
	for _, r := range hits {
		out = append(
			out,
			hit{ID: r.ID, Text: memoryExcerpt(r.Body, keywords), Date: r.Date, Source: r.Source, Tags: r.Tags, Category: r.Category},
		)
	}
	b, _ := json.Marshal(out)
	return tools.SilentResult(string(b))
}

func (t *memorySearchTool) search(ctx context.Context, access privateMemoryAccess, q memory.Search, idArg any) ([]memory.Record, error) {
	authored := access.authored
	if access.management != nil {
		authored = access.management.authored
	}
	ids := memoryID.FindAllStringSubmatch(authored, -1)
	if m := access.management; m != nil && m.intent.existing && len(ids) > 0 &&
		(idArg == nil || len(ids) != 1) {
		return nil, fmt.Errorf("use the one exact ID supplied by the user")
	}
	if idArg != nil {
		var id int64
		raw, err := json.Marshal(idArg)
		if err != nil || json.Unmarshal(raw, &id) != nil || id < 1 {
			return nil, fmt.Errorf("invalid record ID")
		}
		if !slices.ContainsFunc(ids, func(match []string) bool {
			n, _ := strconv.ParseInt(match[1], 10, 64)
			return n == id
		}) {
			return nil, fmt.Errorf("record ID was not supplied by the user")
		}
		r, ok, err := t.db.RecordByID(ctx, id)
		if err != nil || !ok {
			return nil, err
		}
		if m := access.management; m != nil {
			m.markDirect([]memory.Record{r})
		}
		return []memory.Record{r}, nil
	}
	if m := access.management; m != nil && m.intent.existing {
		// A model cannot narrow a destructive lookup with an invented alias,
		// date or tag. The main keyword must occur in the original request.
		if !memorySearchSubject(authored, q.Query) {
			return nil, fmt.Errorf("use the user's subject as the main keyword")
		}
		if !memoryTextGrounded(authored, q.From) {
			q.From = ""
		}
		if !memoryTextGrounded(authored, q.To) {
			q.To = ""
		}
		q.Alias, q.Tag = "", ""
		// A model-suggested alternative may find additional candidates, but
		// only the actual subject can authorize a
		// direct mutation without asking the user to select the result.
		primary := q
		primary.Alternatives = nil
		hits, err := t.db.Search(ctx, primary)
		if err != nil {
			return nil, err
		}
		m.markDirect(slices.DeleteFunc(slices.Clone(hits), func(r memory.Record) bool {
			return !recordMatchesManagementSubject(m, r)
		}))
		if len(q.Alternatives) == 0 {
			return hits, nil
		}
		expanded, err := t.db.Search(ctx, q)
		if err != nil {
			return nil, err
		}
		for _, r := range expanded {
			if !slices.ContainsFunc(hits, func(known memory.Record) bool { return known.ID == r.ID }) {
				hits = append(hits, r)
			}
		}
		return hits[:min(5, len(hits))], nil
	}
	return t.db.Search(ctx, q)
}

func (m *memoryManagement) markDirect(hits []memory.Record) {
	if m.direct == nil {
		m.direct = make(map[int64]bool)
	}
	for _, r := range hits {
		m.direct[r.ID] = true
	}
}

func memorySearchSubject(authored, query string) bool {
	query = strings.TrimSpace(query)
	if !memoryTextGrounded(authored, query) {
		return false
	}
	for _, term := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	}) {
		switch term {
		case "删掉", "删除", "记住", "更正", "修正", "改成", "更新", "上次", "之前", "记的", "记忆", "记录", "清单", "进度", "内容", "上次记的", "那条", "那一条", "之前记的",
			"last", "old", "record", "memory", "note", "it", "that", "the", "checklist", "progress":
		default:
			if len([]rune(term)) >= 2 {
				return true
			}
		}
	}
	return false
}

var managementSubjectFillers = strings.NewReplacer(
	"上次记的", "", "之前记的", "", "最近记的", "", "我记的", "",
	"原来的", "", "已保存的", "", "那条", "", "这条", "", "我的", "",
	"清单", "", "记录", "", "记忆", "", "进度", "", "的", "",
	"the ", "", "my ", "", "from last time", "",
	"record", "", "memory", "", "checklist", "", "progress", "",
)

func normalizedManagementSubject(text string) string {
	text = managementSubjectFillers.Replace(strings.ToLower(text))
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			return -1
		}
		return r
	}, text)
}

// A related search hit is useful as a choice, but insufficient on its own
// to delete something when it omits a qualifier the user actually named.
func recordMatchesManagementSubject(m *memoryManagement, r memory.Record) bool {
	subject := m.authored
	if m.intent.action == "delete" {
		subject = memoryRemove.ReplaceAllString(subject, "")
		subject = regexpDeleteSuffix.ReplaceAllString(subject, "")
	} else {
		subject = naturalRecord.ReplaceAllString(subject, "")
		if m := memoryEdit.FindStringIndex(subject); m != nil && m[0] > 0 {
			subject = subject[:m[0]]
		} else {
			subject = memoryEditLead.ReplaceAllString(subject, "")
			for _, separator := range []string{"更正为", "更新为", "改为", "到", "为", "：", ":", " to "} {
				if before, _, found := strings.Cut(subject, separator); found && before != "" {
					subject = before
				}
			}
		}
	}
	subject = normalizedManagementSubject(subject)
	if len([]rune(subject)) < 2 {
		return false
	}
	labels := strings.Join(append(append([]string{r.Body}, r.Aliases...), r.Tags...), " ")
	return strings.Contains(normalizedManagementSubject(labels), subject)
}

func memoryExcerpt(body, query string) string {
	const limit = 480
	text := []rune(body)
	if len(text) <= limit {
		return body
	}
	start := 0
	for _, keyword := range strings.FieldsFunc(query, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	}) {
		if i := strings.Index(body, keyword); i >= 0 {
			start = min(max(0, len([]rune(body[:i]))-80), len(text)-limit)
			break
		}
	}
	return string(text[start : start+limit])
}
