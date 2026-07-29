package cache

import (
	"testing"

	"conductor/internal/api"
)

func req(model string, msgs ...api.Message) *api.ChatCompletionRequest {
	return &api.ChatCompletionRequest{Model: model, Messages: msgs}
}

func msg(role, content string) api.Message {
	return api.Message{Role: role, Content: content}
}

func TestKey_SameRequestSameKey(t *testing.T) {
	r := req("gpt-4o-mini", msg("user", "hello"))
	if Key(r) != Key(r) {
		t.Fatal("same request produced different keys")
	}
}

func TestKey_DifferentModelDifferentKey(t *testing.T) {
	a := Key(req("gpt-4o-mini", msg("user", "hello")))
	b := Key(req("gpt-4o", msg("user", "hello")))
	if a == b {
		t.Fatal("different models should produce different keys")
	}
}

func TestKey_DifferentContentDifferentKey(t *testing.T) {
	a := Key(req("gpt-4o-mini", msg("user", "hello")))
	b := Key(req("gpt-4o-mini", msg("user", "world")))
	if a == b {
		t.Fatal("different content should produce different keys")
	}
}

func TestKey_ExtraFieldsIgnored(t *testing.T) {
	r1 := &api.ChatCompletionRequest{
		Model:    "gpt-4o-mini",
		Messages: []api.Message{msg("user", "hello")},
		Stream:   true, // not normalised
	}
	r2 := &api.ChatCompletionRequest{
		Model:    "gpt-4o-mini",
		Messages: []api.Message{msg("user", "hello")},
		Stream:   false,
	}
	if Key(r1) != Key(r2) {
		t.Fatal("stream flag should not affect cache key")
	}
}
