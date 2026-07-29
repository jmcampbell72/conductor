// Anthropic Messages API provider. Translates OpenAI-format requests to the
// Anthropic wire format, extracts system messages, and normalises stop reasons
// back to OpenAI conventions.
//
// Author: Justin Campbell
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"conductor/internal/api"
)

const (
	anthropicBase    = "https://api.anthropic.com/v1"
	anthropicVersion = "2023-06-01"
	anthropicDefault = 4096
)

type Anthropic struct {
	key    string
	client *http.Client
}

func NewAnthropic(key string) *Anthropic {
	return &Anthropic{
		key:    key,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *Anthropic) Name() string { return "anthropic" }

type anthropicRequest struct {
	Model       string        `json:"model"`
	MaxTokens   int           `json:"max_tokens"`
	System      string        `json:"system,omitempty"`
	Messages    []api.Message `json:"messages"`
	Temperature *float64      `json:"temperature,omitempty"`
}

type anthropicResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (p *Anthropic) Complete(ctx context.Context, req *api.ChatCompletionRequest) (*api.ChatCompletionResponse, error) {
	aReq := toAnthropicRequest(req)

	body, err := json.Marshal(aReq)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicBase+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.key)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if jsonErr := json.NewDecoder(resp.Body).Decode(&errResp); jsonErr != nil {
			return nil, fmt.Errorf("anthropic: HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("anthropic: %s", errResp.Error.Message)
	}

	var aResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&aResp); err != nil {
		return nil, err
	}
	return fromAnthropicResponse(req.Model, &aResp), nil
}

func toAnthropicRequest(req *api.ChatCompletionRequest) *anthropicRequest {
	aReq := &anthropicRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
	if aReq.MaxTokens == 0 {
		aReq.MaxTokens = anthropicDefault
	}
	for _, m := range req.Messages {
		if m.Role == "system" {
			aReq.System = m.Content
		} else {
			aReq.Messages = append(aReq.Messages, m)
		}
	}
	return aReq
}

func fromAnthropicResponse(model string, aResp *anthropicResponse) *api.ChatCompletionResponse {
	var content string
	for _, block := range aResp.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}
	return &api.ChatCompletionResponse{
		ID:      aResp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []api.ChatCompletionChoice{{
			Index:        0,
			Message:      api.Message{Role: "assistant", Content: content},
			FinishReason: mapStopReason(aResp.StopReason),
		}},
		Usage: api.Usage{
			PromptTokens:     aResp.Usage.InputTokens,
			CompletionTokens: aResp.Usage.OutputTokens,
			TotalTokens:      aResp.Usage.InputTokens + aResp.Usage.OutputTokens,
		},
	}
}

func mapStopReason(r string) string {
	switch r {
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return r
	}
}
