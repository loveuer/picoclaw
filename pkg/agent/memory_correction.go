package agent

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/memory"
)

var (
	correctionVerb     = regexp.MustCompile(`改成|改为|改到|替换成|替换为`)
	correctionOldQuote = regexp.MustCompile(`(?:「([^」]+)」|『([^』]+)』|“([^”]+)”|"([^"]+)")\s*$`)
	// Only a whole affirmative directive with an exact record ID and two
	// literal operands can authorize an immediate correction.
	directCorrection = regexp.MustCompile(
		`^(?:请(?:你|帮我)?\s*)?(?:把|将)\s*(?:记录|记忆)\s*[#＃]([0-9]{1,19})\s*(?:里的|中的|内的)\s*` +
			correctionOperand + `\s*(?:改成|改为|改到|替换成|替换为)\s*` + correctionOperand + `[。！!]?$`,
	)
)

const correctionOperand = `(「[^」\n]+」|『[^』\n]+』|“[^”\n]+”|"[^"\n]+"|[\p{L}\p{N}._/-]{1,40})`

type correctionEvidence struct {
	oldText, newText string
	compound         bool
}

// correctionFromRequest binds simple replacement directives to their operands.
// When the user only describes the desired state, the old text is left unset.
func correctionFromRequest(authored string) correctionEvidence {
	verbs := correctionVerb.FindAllStringIndex(authored, -1)
	if len(verbs) != 1 {
		return correctionEvidence{compound: len(verbs) > 1}
	}
	verb := verbs[0]
	left := strings.TrimSpace(naturalRecord.ReplaceAllString(authored[:verb[0]], ""))
	if memoryClauseSeparator.MatchString(memoryDirectiveText(left)) || strings.ContainsAny(memoryDirectiveText(left), "?？") {
		return correctionEvidence{compound: true}
	}
	right := strings.TrimSpace(authored[verb[1]:])
	right = strings.TrimSpace(strings.TrimSuffix(strings.TrimRight(right, "。！! "), "吧"))
	evidence := correctionEvidence{newText: correctionLiteral(right)}
	if evidence.newText == right && strings.ContainsAny(right, "，,；;。！？!?\n") {
		evidence.newText = ""
		evidence.compound = true
	}
	if quote := correctionOldQuote.FindStringSubmatch(left); quote != nil {
		for _, literal := range quote[1:] {
			if literal != "" {
				evidence.oldText = literal
				return evidence
			}
		}
	}
	for _, separator := range []string{"里的", "中的", "内的", "记录的", "记忆的", "把", "将"} {
		if (separator == "把" || separator == "将") && strings.Count(left, "把")+strings.Count(left, "将") > 1 {
			break
		}
		if i := strings.LastIndex(left, separator); i >= 0 {
			value := correctionLiteral(left[i+len(separator):])
			if value != "" && !strings.ContainsAny(value, "，,；;。！？?\n") &&
				!hasMemoryWords(value, "记录", "记忆", "进度", "正文", "日期", "全文", "整条", "条目", "清单", "偏好") {
				evidence.oldText = value
				break
			}
		}
	}
	return evidence
}

func correctionLiteral(text string) string {
	text = strings.TrimSpace(text)
	for _, quotes := range [][2]string{{"「", "」"}, {"『", "』"}, {"“", "”"}, {"\"", "\""}} {
		if strings.HasPrefix(text, quotes[0]) && strings.HasSuffix(text, quotes[1]) && len(text) >= len(quotes[0])+len(quotes[1]) {
			return text[len(quotes[0]) : len(text)-len(quotes[1])]
		}
	}
	return text
}

func matchesDirectCorrection(authored string, id int64, old, replacement string) bool {
	match := directCorrection.FindStringSubmatch(strings.TrimSpace(authored))
	directive := memoryDirectiveText(authored)
	if match == nil || memoryNonDirective.MatchString(directive) || memoryFollowUp.MatchString(directive) {
		return false
	}
	target, err := strconv.ParseInt(match[1], 10, 64)
	return err == nil && target == id && correctionLiteral(match[2]) == old && correctionLiteral(match[3]) == replacement
}

func uniqueCorrectionText(body, value string) bool {
	at := strings.Index(body, value)
	return at >= 0 && at == strings.LastIndex(body, value)
}

type proposedMemoryCorrection struct {
	record memory.Record
	patch  memory.Record
}

func (p proposedMemoryCorrection) question() string {
	lines := []string{fmt.Sprintf("请核对记录 #%d 的更正。", p.record.ID)}
	if p.record.Body != p.patch.Body {
		lines = append(lines, fmt.Sprintf("原文 %q\n更正后 %q", p.record.Body, p.patch.Body))
	}
	if p.patch.Date != "" && p.patch.Date != p.record.Date {
		lines = append(lines, fmt.Sprintf("日期 %s → %s", p.record.Date, p.patch.Date))
	}
	lines = append(lines, "回复「确认更正」应用这项改动，或回复「取消更正」。")
	return strings.Join(lines, "\n")
}

func (t *memoryWriteTool) correctRecord(ctx context.Context, m *memoryManagement, r memory.Record, a memoryWriteArgs) (string, error) {
	evidence := correctionFromRequest(m.authored)
	proposal := proposedMemoryCorrection{record: r, patch: memory.Record{Body: r.Body, Date: a.Date}}
	if a.Text != "" {
		if !memoryTextGrounded(m.authored, a.Text) || (evidence.newText != "" && evidence.newText != a.Text) {
			return "更正未执行。请按「把旧值改成新值」说明完整替换内容。", nil
		}
		if a.OldText != "" {
			replacement := a.Text
			if evidence.oldText != "" && evidence.oldText != a.OldText {
				// A larger, unique excerpt may locate a repeated old value.
				// Change only the value the user named within that excerpt.
				if !uniqueCorrectionText(a.OldText, evidence.oldText) {
					return "更正未执行。找到的旧值与原话不符，请说明要替换的原文。", nil
				}
				replacement = strings.Replace(a.OldText, evidence.oldText, a.Text, 1)
			}
			if !uniqueCorrectionText(r.Body, a.OldText) {
				return "", nil
			}
			proposal.patch.Body = strings.Replace(r.Body, a.OldText, replacement, 1)
		} else {
			if evidence.oldText != "" || !hasMemoryWords(strings.ToLower(m.authored),
				"正文", "全文", "整条", "entire record", "full text", "body to") {
				return "", nil
			}
			proposal.patch.Body = a.Text
		}
	} else if a.OldText != "" || a.Date == "" || evidence.oldText != "" {
		return "", nil
	}
	if proposal.patch.Body == r.Body && (a.Date == "" || a.Date == r.Date) {
		return "这条记录已经是指定内容。", nil
	}
	if !evidence.compound && evidence.oldText != "" && evidence.oldText == a.OldText &&
		evidence.newText != "" && evidence.newText == a.Text && a.Date == "" &&
		matchesDirectCorrection(m.authored, r.ID, a.OldText, a.Text) {
		return applyMemoryCorrection(ctx, t.agent.memoryDB, m.recordMatch(r), proposal)
	}
	m.correction = &proposal
	return proposal.question(), nil
}

func applyMemoryCorrection(ctx context.Context, db *memory.SQLite, match memory.RecordMatch, p proposedMemoryCorrection) (string, error) {
	ok, err := db.UpdateRecordMatch(ctx, p.record.ID, match, p.patch)
	if err != nil {
		return "", err
	}
	if !ok {
		return "这条记录或会话已变更，请重新发起更正。", nil
	}
	return fmt.Sprintf("已更正记录 #%d。", p.record.ID), nil
}

func (al *AgentLoop) finishMemoryCorrection(ctx context.Context, db *memory.SQLite, key, action string) string {
	raw, ok := al.pendingMemoryChoices.Load(key)
	if !ok {
		return "当前会话没有待确认的更正。"
	}
	pending := raw.(*pendingMemoryChoice)
	if pending.management.correction == nil {
		return "当前会话没有待确认的更正。"
	}
	if !al.pendingMemoryChoices.CompareAndDelete(key, raw) || !time.Now().Before(pending.expires) {
		return "更正预览已过期，请重新发起更正。"
	}
	if action == "取消更正" {
		return "已取消更正。"
	}
	proposal := *pending.management.correction
	reply, err := applyMemoryCorrection(ctx, db, memory.RecordMatch{
		Version: proposal.record.UpdatedAt, SessionKey: key, SessionRevision: pending.management.revision,
	}, proposal)
	if err != nil {
		return "更正写入失败。"
	}
	return reply
}
