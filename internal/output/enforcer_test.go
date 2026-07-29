// Tests for the output enforcer: max_tokens cap, response_format injection,
// concise-response instruction placement, and immutability guarantees.
//
// Author: Justin Campbell
package output

import (
	"testing"

	"conductor/internal/api"
	"conductor/internal/config"
)

func route(opts ...func(*config.RouteConfig)) config.RouteConfig {
	r := config.RouteConfig{}
	for _, o := range opts {
		o(&r)
	}
	return r
}

func withMaxTokens(n int) func(*config.RouteConfig) {
	return func(r *config.RouteConfig) { r.MaxTokens = n }
}

func withFormat(t string) func(*config.RouteConfig) {
	return func(r *config.RouteConfig) { r.ResponseFormatType = t }
}

func withConcise() func(*config.RouteConfig) {
	return func(r *config.RouteConfig) { r.ConciseResponse = true }
}

func req(maxTokens int, msgs ...api.Message) *api.ChatCompletionRequest {
	return &api.ChatCompletionRequest{
		Model:     "gpt-4o-mini",
		Messages:  msgs,
		MaxTokens: maxTokens,
	}
}

func msg(role, content string) api.Message {
	return api.Message{Role: role, Content: content}
}

var e = New()

// ── max_tokens ────────────────────────────────────────────────────────────────

func TestMaxTokens_CapAppliedWhenCallerUnset(t *testing.T) {
	out := e.Apply(req(0, msg("user", "hi")), route(withMaxTokens(500)))
	if out.MaxTokens != 500 {
		t.Errorf("got %d, want 500", out.MaxTokens)
	}
}

func TestMaxTokens_CapAppliedWhenCallerExceeds(t *testing.T) {
	out := e.Apply(req(2000, msg("user", "hi")), route(withMaxTokens(500)))
	if out.MaxTokens != 500 {
		t.Errorf("got %d, want 500", out.MaxTokens)
	}
}

func TestMaxTokens_CallerValuePreservedWhenUnderCap(t *testing.T) {
	out := e.Apply(req(100, msg("user", "hi")), route(withMaxTokens(500)))
	if out.MaxTokens != 100 {
		t.Errorf("got %d, want 100", out.MaxTokens)
	}
}

func TestMaxTokens_NoCapWhenRouteUnset(t *testing.T) {
	out := e.Apply(req(0, msg("user", "hi")), route())
	if out.MaxTokens != 0 {
		t.Errorf("got %d, want 0", out.MaxTokens)
	}
}

// ── response_format ───────────────────────────────────────────────────────────

func TestResponseFormat_AppliedWhenCallerAbsent(t *testing.T) {
	out := e.Apply(req(0, msg("user", "hi")), route(withFormat("json_object")))
	if out.ResponseFormat == nil || out.ResponseFormat.Type != "json_object" {
		t.Errorf("expected json_object format, got %v", out.ResponseFormat)
	}
}

func TestResponseFormat_CallerFormatPreserved(t *testing.T) {
	r := req(0, msg("user", "hi"))
	r.ResponseFormat = &api.ResponseFormat{Type: "text"}
	out := e.Apply(r, route(withFormat("json_object")))
	if out.ResponseFormat.Type != "text" {
		t.Errorf("caller format overwritten, got %q", out.ResponseFormat.Type)
	}
}

func TestResponseFormat_NilWhenRouteUnset(t *testing.T) {
	out := e.Apply(req(0, msg("user", "hi")), route())
	if out.ResponseFormat != nil {
		t.Errorf("expected nil response_format, got %v", out.ResponseFormat)
	}
}

// ── concise_response ──────────────────────────────────────────────────────────

func TestConcise_InjectedBeforeUserWhenNoSystemExists(t *testing.T) {
	out := e.Apply(req(0, msg("user", "explain something")), route(withConcise()))
	if len(out.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(out.Messages))
	}
	if out.Messages[0].Role != "system" || out.Messages[0].Content != conciseInstruction {
		t.Errorf("first message is not the concise instruction: %+v", out.Messages[0])
	}
	if out.Messages[1].Role != "user" {
		t.Errorf("second message should be user, got %q", out.Messages[1].Role)
	}
}

func TestConcise_InjectedAfterCallerSystemMessage(t *testing.T) {
	out := e.Apply(req(0,
		msg("system", "You are a helpful assistant."),
		msg("user", "explain something"),
	), route(withConcise()))

	if len(out.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(out.Messages))
	}
	if out.Messages[0].Content != "You are a helpful assistant." {
		t.Errorf("caller system message moved: %q", out.Messages[0].Content)
	}
	if out.Messages[1].Content != conciseInstruction {
		t.Errorf("concise instruction not in second position: %q", out.Messages[1].Content)
	}
	if out.Messages[2].Role != "user" {
		t.Errorf("user message not in third position: %q", out.Messages[2].Role)
	}
}

func TestConcise_NotInjectedWhenDisabled(t *testing.T) {
	out := e.Apply(req(0, msg("user", "hi")), route())
	if len(out.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(out.Messages))
	}
}

// ── immutability ──────────────────────────────────────────────────────────────

func TestApply_DoesNotMutateOriginal(t *testing.T) {
	original := req(2000, msg("user", "hi"))
	_ = e.Apply(original, route(withMaxTokens(500), withConcise()))
	if original.MaxTokens != 2000 {
		t.Errorf("original MaxTokens mutated: got %d", original.MaxTokens)
	}
	if len(original.Messages) != 1 {
		t.Errorf("original Messages mutated: got %d messages", len(original.Messages))
	}
}
