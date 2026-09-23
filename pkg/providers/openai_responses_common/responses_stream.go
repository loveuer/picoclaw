package openai_responses_common

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/sipeed/picoclaw/pkg/providers/protocoltypes"
)

// ParseResponseStream reads a Responses API event stream and returns its final
// response. Some compatible endpoints require stream=true even for Chat calls.
func ParseResponseStream(body io.Reader) (*protocoltypes.LLMResponse, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 32*1024*1024)
	var eventType string
	var data strings.Builder
	var streamedText strings.Builder
	var outputItems []json.RawMessage

	processEvent := func() (*protocoltypes.LLMResponse, bool, error) {
		payload := strings.TrimSpace(data.String())
		if payload == "" {
			return nil, false, nil
		}
		if payload == "[DONE]" {
			return nil, true, fmt.Errorf("responses stream ended without a final response")
		}

		var event struct {
			Type     string          `json:"type"`
			Response json.RawMessage `json:"response"`
			Delta    string          `json:"delta"`
			Text     string          `json:"text"`
			Item     json.RawMessage `json:"item"`
			Error    struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return nil, true, fmt.Errorf("invalid responses stream event: %w", err)
		}
		if event.Type == "" {
			event.Type = eventType
		}
		switch event.Type {
		case "response.completed", "response.incomplete", "response.failed":
			if len(event.Response) == 0 {
				return nil, true, fmt.Errorf("responses stream %s event has no response", event.Type)
			}
			responseJSON := event.Response
			if len(outputItems) > 0 {
				var body map[string]json.RawMessage
				if err := json.Unmarshal(responseJSON, &body); err != nil {
					return nil, true, fmt.Errorf("invalid final responses stream body: %w", err)
				}
				output := bytes.TrimSpace(body["output"])
				if len(output) == 0 || bytes.Equal(output, []byte("null")) || bytes.Equal(output, []byte("[]")) {
					itemsJSON, err := json.Marshal(outputItems)
					if err != nil {
						return nil, true, fmt.Errorf("encoding responses stream output items: %w", err)
					}
					body["output"] = itemsJSON
					responseJSON, err = json.Marshal(body)
					if err != nil {
						return nil, true, fmt.Errorf("encoding final responses stream body: %w", err)
					}
				}
			}
			response, err := ParseResponseBody(bytes.NewReader(responseJSON))
			if err == nil && response.Content == "" && streamedText.Len() > 0 {
				response.Content = streamedText.String()
			}
			return response, true, err
		case "error":
			if event.Error.Message != "" {
				return nil, true, fmt.Errorf("responses stream error: %s", event.Error.Message)
			}
			return nil, true, fmt.Errorf("responses stream returned an error event")
		case "response.output_text.delta":
			streamedText.WriteString(event.Delta)
		case "response.output_text.done":
			if streamedText.Len() == 0 {
				streamedText.WriteString(event.Text)
			}
		case "response.output_item.done":
			if len(event.Item) > 0 {
				outputItems = append(outputItems, event.Item)
			}
		}
		return nil, false, nil
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			response, done, err := processEvent()
			if done {
				return response, err
			}
			data.Reset()
			eventType = ""
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading responses stream: %w", err)
	}
	if response, done, err := processEvent(); done {
		return response, err
	}
	return nil, fmt.Errorf("responses stream ended without a final response")
}
