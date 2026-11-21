package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/memory"
)

var (
	naturalRecord  = regexp.MustCompile(`^(?:请\s*)?(?:(?:帮我\s*)?(?:记录一下|记住|记下|记一下)|帮我\s*记录|记录[：:，,、\s]+)[：:，,、\s]*`)
	structuredDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	leadingDate    = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})(?:\s|[：:，,]|$)`)
)

func memoryCommand(text string) (string, string, bool) {
	text = strings.TrimSpace(text)
	for _, prefix := range []string{"/memory", "/preference"} {
		if text == prefix {
			return prefix, "", true
		}
		if strings.HasPrefix(text, prefix+" ") || strings.HasPrefix(text, prefix+"\n") ||
			strings.HasPrefix(text, prefix+"\t") {
			return prefix, strings.TrimSpace(strings.TrimPrefix(text, prefix)), true
		}
	}
	intent := detectMemoryIntent(text)
	if intent.kind == "preference" || intent.existing {
		return "", "", false
	}
	if prefix := naturalRecord.FindString(text); prefix != "" {
		directive := memoryDirectiveText(text)
		if memoryFollowUp.MatchString(directive) ||
			(memorySaved.MatchString(text) && memoryEdit.MatchString(text) &&
				(memoryNonDirective.MatchString(directive) || correctionFromRequest(text).compound)) {
			return "", "", false
		}
		body := strings.TrimSpace(strings.TrimPrefix(text, prefix))
		if !strings.HasSuffix(body, "?") && !strings.HasSuffix(body, "？") && !strings.HasPrefix(body, "“") &&
			!strings.HasPrefix(body, "\"") &&
			!strings.HasPrefix(body, ">") {
			return "/memory", "add " + body, true
		}
	}
	return "", "", false
}

func firstArgument(s string) (string, string) {
	i := strings.IndexAny(s, " \t\r\n")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i+1:])
}

func memoryPositiveID(s string) (int64, error) {
	n, e := strconv.ParseInt(s, 10, 64)
	if e != nil || n < 1 {
		return 0, fmt.Errorf("需要有效的记录编号")
	}
	return n, nil
}

func parseRecordInput(raw string) (memory.Record, error) {
	r := memory.Record{Body: raw}
	if strings.HasPrefix(raw, "{") {
		var v struct {
			Text     string   `json:"text"`
			Date     string   `json:"date"`
			Aliases  []string `json:"aliases"`
			Tags     []string `json:"tags"`
			Category string   `json:"category"`
		}
		d := json.NewDecoder(strings.NewReader(raw))
		d.DisallowUnknownFields()
		if e := d.Decode(&v); e != nil || !json.Valid([]byte(raw)) {
			return r, fmt.Errorf("无效记录 JSON：需要 text，可选 date、aliases、tags、category")
		}
		r.Body, r.Date, r.Aliases, r.Tags, r.Category = v.Text, v.Date, v.Aliases, v.Tags, strings.TrimSpace(v.Category)
	} else if arg, next := firstArgument(raw); arg == "--date" {
		r.Date, r.Body = firstArgument(next)
		if r.Date == "" {
			return r, fmt.Errorf("--date 需要 YYYY-MM-DD")
		}
	} else if m := leadingDate.FindStringSubmatch(raw); len(m) > 0 {
		r.Date = m[1]
	}
	if r.Date != "" && !structuredDate.MatchString(r.Date) {
		return r, fmt.Errorf("日期必须为 YYYY-MM-DD")
	}
	return r, nil
}

func (agent *AgentInstance) handlePrivateMemory(
	ctx context.Context,
	allowed bool,
	text, authored, source string,
) (string, bool, error) {
	// Read and write commands use trusted authored text (or the configured
	// terminal), never content reconstructed by media, replies or the model.
	kind, args, handled := memoryCommand(authored)
	if !handled {
		if _, _, explicit := memoryCommand(text); explicit {
			return "这条内容没有保存。请在本地终端或通过访问规则的飞书或微信私聊直接输入。", true, nil
		}
		return "", false, nil
	}
	action, rest := firstArgument(args)
	db := agent.memoryDB
	if !allowed || db == nil {
		return "当前会话无法访问实例的私人记忆。", true, nil
	}
	if kind == "/preference" {
		switch action {
		case "list":
			list, e := db.Preferences(ctx)
			if e != nil {
				return "", true, e
			}
			parts := []string{}
			for _, p := range list {
				parts = append(parts, p.Key+": "+p.Value)
			}
			if len(parts) == 0 {
				return "没有已保存的偏好。", true, nil
			}
			return strings.Join(parts, "\n"), true, nil
		case "set":
			k, v := firstArgument(rest)
			if e := db.SetPreference(ctx, k, v, source); e != nil {
				return "", true, e
			}
			return "已保存偏好：" + k, true, nil
		case "delete":
			ok, e := db.DeletePreference(ctx, rest)
			if e != nil {
				return "", true, e
			}
			if !ok {
				return "没有该偏好。", true, nil
			}
			return "偏好已删除。", true, nil
		}
		return "用法：/preference list | set <key> <value> | delete <key>", true, nil
	}
	switch action {
	case "add":
		if rest == "" {
			return "请告诉我要保存什么，例如：帮我记一下，原神玩到第三章。", true, nil
		}
		r, e := parseRecordInput(rest)
		if e != nil {
			return "", true, e
		}
		r.Source = source
		reply, e := agent.saveImmediateRecord(ctx, r)
		return reply, true, e
	case "update", "delete", "export":
		arg, body := firstArgument(rest)
		var id int64
		if action != "export" || arg != "" {
			var e error
			id, e = memoryPositiveID(arg)
			if e != nil {
				return "", true, e
			}
		}
		if action == "export" {
			rs, e := db.Export(ctx, id)
			if e != nil {
				return "", true, e
			}
			if len(rs) == 0 {
				return "没有更多记录。", true, nil
			}
			b, _ := json.MarshalIndent(rs[0], "", "  ")
			return fmt.Sprintf("%s\n\n继续导出：/memory export %d", b, rs[0].ID), true, nil
		}
		var ok bool
		var e error
		if action == "delete" {
			ok, e = db.Delete(ctx, id)
		} else {
			r, parseErr := parseRecordInput(body)
			if parseErr != nil {
				return "", true, parseErr
			}
			ok, e = db.Update(ctx, id, r)
		}
		if e != nil {
			return "", true, e
		}
		if !ok {
			return "没有找到该记录。", true, nil
		}
		if action == "delete" {
			return "记录已删除。", true, nil
		}
		return "记录已更正。", true, nil
	}
	return "用法：/memory add <正文> | update <id> <正文> | delete <id> | export [cursor]", true, nil
}

func fillMemoryDate(ctx context.Context, db *memory.SQLite, r *memory.Record) error {
	if r.Date != "" {
		return nil
	}
	loc, err := db.Location(ctx)
	if err != nil {
		return err
	}
	now := time.Now().In(loc)
	body := strings.TrimPrefix(r.Body, "我")
	switch {
	case strings.HasPrefix(body, "昨天"):
		now = now.AddDate(0, 0, -1)
	case strings.HasPrefix(body, "前天"):
		now = now.AddDate(0, 0, -2)
	}
	r.Date = now.Format(time.DateOnly)
	return nil
}
