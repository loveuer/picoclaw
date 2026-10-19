package commands

import (
	"context"
	"strings"
	"unicode"
)

func newCommand() Definition {
	return Definition{
		Name:        "new",
		Description: "Clear the chat history and optionally run a new prompt",
		Usage:       "/new [prompt]",
		Handler: func(ctx context.Context, req Request, rt *Runtime) error {
			if rt == nil || rt.StartNewConversation == nil {
				return req.Reply(unavailableMsg)
			}
			text := strings.TrimSpace(req.Text)
			var prompt string
			if i := strings.IndexFunc(text, unicode.IsSpace); i >= 0 {
				prompt = strings.TrimSpace(text[i:])
			}
			response, err := rt.StartNewConversation(ctx, prompt)
			if err != nil {
				return err
			}
			return req.Reply(response)
		},
	}
}
