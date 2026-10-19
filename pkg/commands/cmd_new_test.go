package commands

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewCommand(t *testing.T) {
	for _, tt := range []struct{ input, prompt string }{
		{"/new", ""},
		{" /new \t\n", ""},
		{"/new 帮我查查今天成都的天气", "帮我查查今天成都的天气"},
		{"/new 看看今天有啥新咖啡", "看看今天有啥新咖啡"},
		{"/NEW@testbot\t first  line\nsecond\tline  ", "first  line\nsecond\tline"},
		{"!new\n问题", "问题"},
		{"/new /clear", "/clear"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			calls := 0
			rt := &Runtime{StartNewConversation: func(ctx context.Context, prompt string) (string, error) {
				calls++
				if prompt != tt.prompt {
					t.Fatalf("prompt = %q, want %q", prompt, tt.prompt)
				}
				return "new reply", nil
			}}
			var reply string
			res := NewExecutor(NewRegistry(BuiltinDefinitions()), rt).Execute(t.Context(), Request{
				Text:  tt.input,
				Reply: func(text string) error { reply = text; return nil },
			})
			if res.Outcome != OutcomeHandled || res.Err != nil || calls != 1 || reply != "new reply" {
				t.Fatalf("result = %+v, calls = %d, reply = %q", res, calls, reply)
			}
		})
	}
}

func TestNewCommand_UnavailableAndFailure(t *testing.T) {
	for _, rt := range []*Runtime{nil, {}} {
		var reply string
		res := NewExecutor(NewRegistry(BuiltinDefinitions()), rt).Execute(t.Context(), Request{
			Text:  "/new question",
			Reply: func(text string) error { reply = text; return nil },
		})
		if res.Outcome != OutcomeHandled || reply != unavailableMsg {
			t.Fatalf("result = %+v, reply = %q", res, reply)
		}
	}
	wantErr := errors.New("clear failed")
	res := NewExecutor(NewRegistry(BuiltinDefinitions()), &Runtime{
		StartNewConversation: func(context.Context, string) (string, error) { return "", wantErr },
	}).Execute(t.Context(), Request{Text: "/new question"})
	if res.Outcome != OutcomeHandled || !errors.Is(res.Err, wantErr) {
		t.Fatalf("result = %+v, want clear error", res)
	}
}

func TestNewCommand_Help(t *testing.T) {
	if help := formatHelpMessage(BuiltinDefinitions()); !strings.Contains(help, "/new [prompt]") {
		t.Fatalf("help missing /new: %s", help)
	}
}
