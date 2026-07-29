// Tests for the complexity scorer and task type classifier.
//
// Author: Justin Campbell
package router

import (
	"testing"

	"conductor/internal/api"
)

func req(content string) *api.ChatCompletionRequest {
	return &api.ChatCompletionRequest{
		Messages: []api.Message{{Role: "user", Content: content}},
	}
}

func TestScore_SimpleGreeting(t *testing.T) {
	score := NewAnalyzer().Analyze(req("hi there")).Score
	if score >= 0.3 {
		t.Fatalf("greeting should score low; got %.2f", score)
	}
}

func TestScore_TechnicalKeywords(t *testing.T) {
	score := NewAnalyzer().Analyze(req("analyze and implement an optimized algorithm for this problem")).Score
	if score < 0.3 {
		t.Fatalf("technical request should score higher; got %.2f", score)
	}
}

func TestScore_CodeBlock(t *testing.T) {
	score := NewAnalyzer().Analyze(req("review this:\n```go\nfunc main() {}\n```")).Score
	if score < 0.15 {
		t.Fatalf("code block should contribute to score; got %.2f", score)
	}
}

func TestScore_LongMultiTurn(t *testing.T) {
	// Realistic: 10-turn conversation, ~400 chars per message of technical content.
	content := "analyze and design this system architecture in detail, considering " +
		"performance tradeoffs, security implications, and infrastructure constraints " +
		"for a high-throughput production environment. evaluate the database schema " +
		"and identify any vulnerabilities in the current implementation approach"
	messages := make([]api.Message, 10)
	for i := range messages {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		messages[i] = api.Message{Role: role, Content: content}
	}
	score := NewAnalyzer().Analyze(&api.ChatCompletionRequest{Messages: messages}).Score
	if score < 0.6 {
		t.Fatalf("long multi-turn technical conversation should score >= 0.6; got %.2f", score)
	}
}

func TestScore_Bounded(t *testing.T) {
	// Throw everything at it; should never exceed 1.0
	content := "analyze implement debug optimize refactor architect synthesize evaluate " +
		string(make([]byte, 10000)) + "\n```\ncode\n```\n$$math$$"
	messages := make([]api.Message, 20)
	for i := range messages {
		messages[i] = api.Message{Role: "user", Content: content}
	}
	score := NewAnalyzer().Analyze(&api.ChatCompletionRequest{Messages: messages}).Score
	if score > 1.0 {
		t.Fatalf("score must not exceed 1.0; got %.2f", score)
	}
}
