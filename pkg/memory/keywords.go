package memory

import (
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxSearchAlternatives = 4
	maxRecordLabels       = 12
)

func validateCategory(category string) error {
	if category == "" {
		return nil
	}
	parts := strings.Split(category, "/")
	if len(parts) > 3 || utf8.RuneCountInString(category) > 48 {
		return errors.New("category must have one to three levels and at most 48 characters")
	}
	for _, part := range parts {
		if part == "" || part != strings.TrimSpace(part) || strings.ContainsFunc(part, unicode.IsControl) {
			return errors.New("invalid category path; use a path such as 日常/游戏")
		}
	}
	return nil
}

// enrichRecordLabels is intentionally conservative for immediate saves: it
// doesn't wait for a model. The scheduled review can supply other categories
// and aliases directly from the conversation.
func enrichRecordLabels(r Record) Record {
	text := strings.ToLower(r.Body + " " + strings.Join(r.Aliases, " "))
	switch {
	case containsKeyword(text, "原神", "genshin"):
		r.Aliases = addLabels(r.Aliases, "Genshin Impact")
	case containsKeyword(text, "旷野之息", "breath of the wild", "botw"):
		r.Aliases = addLabels(r.Aliases, "旷野之息", "BOTW", "Breath of the Wild")
	}
	for _, subject := range []string{"宝箱", "神庙"} {
		if strings.Contains(text, subject) {
			r.Tags = addLabels(r.Tags, subject)
		}
	}
	if r.Category != "" {
		return r
	}
	switch {
	case containsKeyword(text, "原神", "genshin", "塞尔达", "zelda", "旷野之息", "botw", "breath of the wild", "游戏进度", "游戏存档", "游戏通关") ||
		(strings.Contains(text, "宝箱") && strings.Contains(text, "神庙")):
		r.Category = "日常/游戏"
	case containsKeyword(text, "读书进度", "阅读进度", "读书笔记", "读完了《"):
		r.Category = "日常/阅读"
	case containsKeyword(text, "旅行游记", "旅行行程", "旅游行程"):
		r.Category = "日常/旅行"
	}
	return r
}

func containsKeyword(text string, keywords ...string) bool {
	return slices.ContainsFunc(keywords, func(word string) bool { return strings.Contains(text, word) })
}

func addLabels(existing []string, words ...string) []string {
	result := append([]string(nil), existing...)
	for _, word := range words {
		if len(result) >= maxRecordLabels {
			break
		}
		if !slices.ContainsFunc(result, func(v string) bool { return strings.EqualFold(word, v) }) {
			result = append(result, word)
		}
	}
	return result
}

// Queries returns the main query and its independent, deduplicated alternatives.
func (q Search) Queries() []string {
	var queries []string
	for _, raw := range append([]string{q.Query}, q.Alternatives[:min(len(q.Alternatives), MaxSearchAlternatives)]...) {
		term := strings.TrimSpace(raw)
		if term != "" && !slices.ContainsFunc(queries, func(existing string) bool { return strings.EqualFold(existing, term) }) {
			queries = append(queries, term)
		}
	}
	return queries
}
