package trim

import (
	"context"
	"testing"

	"conductor/internal/api"
)

func msgs(pairs ...string) []api.Message {
	out := make([]api.Message, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, api.Message{Role: pairs[i], Content: pairs[i+1]})
	}
	return out
}

func TestProcess_WithinBudget(t *testing.T) {
	req := &api.ChatCompletionRequest{Messages: msgs("user", "hi")}
	m := New(nil)
	got, err := m.Process(context.Background(), req, 4000)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("want 1 message, got %d", len(got.Messages))
	}
}

func TestProcess_ZeroBudgetPassThrough(t *testing.T) {
	long := string(make([]byte, 10000))
	req := &api.ChatCompletionRequest{Messages: msgs("user", long)}
	m := New(nil)
	got, _ := m.Process(context.Background(), req, 0)
	if got != req {
		t.Fatal("zero budget should return original request unchanged")
	}
}

func TestSlideWindow_PreservesSystemMessages(t *testing.T) {
	req := &api.ChatCompletionRequest{
		Messages: msgs(
			"system", "you are a helpful assistant",
			"user", string(make([]byte, 1000)),
			"assistant", string(make([]byte, 1000)),
			"user", "new question",
		),
	}
	trimmed, dropped := slideWindow(req, 300)
	for _, m := range trimmed.Messages {
		if m.Role == "system" {
			goto found
		}
	}
	t.Fatal("system message was dropped")
found:
	if len(dropped) == 0 {
		t.Fatal("expected messages to be dropped")
	}
}

func TestSlideWindow_KeepsNewest(t *testing.T) {
	req := &api.ChatCompletionRequest{
		Messages: msgs(
			"user", string(make([]byte, 500)),
			"assistant", string(make([]byte, 500)),
			"user", "final question",
		),
	}
	trimmed, _ := slideWindow(req, 50)
	last := trimmed.Messages[len(trimmed.Messages)-1]
	if last.Content != "final question" {
		t.Fatalf("newest message not preserved; got role=%s", last.Role)
	}
}
