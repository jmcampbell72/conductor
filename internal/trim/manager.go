package trim

import (
	"context"
	"fmt"
	"strings"

	"conductor/internal/api"
	"conductor/internal/provider"
)

// Manager trims conversation history to fit within a token budget,
// optionally summarising dropped messages via an LLM.
type Manager struct {
	p provider.Provider // may be nil; used only for summarisation
}

func New(p provider.Provider) *Manager {
	return &Manager{p: p}
}

// Process returns a copy of req with history trimmed to fit within budget tokens.
// A budget of 0 disables trimming (pass-through).
// If messages are dropped and a provider is available, it summarises them and
// injects the summary as a system message so downstream context is preserved.
func (m *Manager) Process(ctx context.Context, req *api.ChatCompletionRequest, budget int) (*api.ChatCompletionRequest, error) {
	if budget <= 0 || EstimateRequest(req) <= budget {
		return req, nil
	}

	trimmed, dropped := slideWindow(req, budget)

	if len(dropped) > 0 && m.p != nil {
		if summary, err := m.summarise(ctx, dropped); err == nil && summary != "" {
			trimmed = injectSummary(trimmed, summary)
		}
	}

	return trimmed, nil
}

// slideWindow removes the oldest non-system messages until the request fits
// within budget. System messages are always preserved.
func slideWindow(req *api.ChatCompletionRequest, budget int) (trimmed *api.ChatCompletionRequest, dropped []api.Message) {
	var sys, conv []api.Message
	for _, m := range req.Messages {
		if m.Role == "system" {
			sys = append(sys, m)
		} else {
			conv = append(conv, m)
		}
	}

	sysTokens := 0
	for _, m := range sys {
		sysTokens += EstimateMessage(m)
	}
	convBudget := budget - sysTokens

	kept := make([]api.Message, 0, len(conv))
	used := 0
	for i := len(conv) - 1; i >= 0; i-- {
		t := EstimateMessage(conv[i])
		if used+t > convBudget {
			dropped = conv[:i+1]
			break
		}
		kept = append([]api.Message{conv[i]}, kept...)
		used += t
	}

	out := *req
	out.Messages = append(sys, kept...)
	return &out, dropped
}

// injectSummary inserts a summary system message immediately after any existing
// system messages, before the retained conversation turns.
func injectSummary(req *api.ChatCompletionRequest, summary string) *api.ChatCompletionRequest {
	var sys, rest []api.Message
	for _, m := range req.Messages {
		if m.Role == "system" {
			sys = append(sys, m)
		} else {
			rest = append(rest, m)
		}
	}
	summaryMsg := api.Message{
		Role:    "system",
		Content: "[Summary of earlier conversation] " + summary,
	}
	out := *req
	out.Messages = append(sys, append([]api.Message{summaryMsg}, rest...)...)
	return &out
}

func (m *Manager) summarise(ctx context.Context, msgs []api.Message) (string, error) {
	var sb strings.Builder
	for _, msg := range msgs {
		fmt.Fprintf(&sb, "[%s]: %s\n", msg.Role, msg.Content)
	}

	resp, err := m.p.Complete(ctx, &api.ChatCompletionRequest{
		Model:     "gpt-4o-mini",
		MaxTokens: 200,
		Messages: []api.Message{
			{
				Role:    "system",
				Content: "Summarise the following conversation history in 2-3 sentences. Preserve key decisions, facts, and context.",
			},
			{Role: "user", Content: sb.String()},
		},
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return resp.Choices[0].Message.Content, nil
}
