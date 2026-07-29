package router

import (
	"strings"

	"conductor/internal/api"
)

// TaskType classifies the nature of the work being requested.
type TaskType string

const (
	TaskGeneral  TaskType = "general"
	TaskPlanning TaskType = "planning"
	TaskWriting  TaskType = "writing"
	TaskQA       TaskType = "qa"
)

// Analysis is the combined output of the analyzer.
type Analysis struct {
	Score    float64
	TaskType TaskType
}

// complexKeywords are signals that a request requires substantive reasoning,
// multi-step generation, or deep domain knowledge.
var complexKeywords = []string{
	"analyze", "analyse", "implement", "debug", "optimize", "optimise",
	"refactor", "architect", "synthesize", "synthesise", "evaluate",
	"compare", "contrast", "critique", "design", "develop", "generate",
	"algorithm", "complexity", "performance", "security", "vulnerability",
	"infrastructure", "database", "schema", "function", "interface",
	"explain", "step by step", "in detail", "how does", "why does",
	"pros and cons", "trade-offs", "tradeoffs",
}

var planningKeywords = []string{
	"plan", "outline", "roadmap", "strategy", "structure", "breakdown",
	"schedule", "milestone", "sprint", "agenda", "timeline", "prioritize",
	"prioritise", "architecture", "design", "proposal",
}

var writingKeywords = []string{
	"write", "draft", "compose", "essay", "blog", "article", "email",
	"letter", "story", "summarize", "summarise", "rewrite", "edit",
	"paragraph", "caption", "copywrite", "narrative", "report",
}

var qaKeywords = []string{
	"test", "verify", "check", "validate", "review", "debug", "fix",
	"bug", "error", "issue", "problem", "assert", "coverage", "qa",
	"quality", "regression", "unittest", "unit test",
}

// Analyzer scores requests on a 0.0–1.0 scale using four heuristic signals
// and classifies the task type from keyword sets.
type Analyzer struct{}

func NewAnalyzer() *Analyzer { return &Analyzer{} }

// Analyze returns a complexity score in [0.0, 1.0] and a task type classification.
// Four signals, with maximum contributions that sum to 1.0:
//
//	Length      0.25  saturates at 5000 characters
//	Turn count  0.15  saturates at 10 messages
//	Keywords    0.35  saturates at 4 matched keywords
//	Structure   0.25  code fences (0.15) + equations/JSON (up to 0.10)
func (a *Analyzer) Analyze(req *api.ChatCompletionRequest) Analysis {
	combined := combinedContent(req.Messages)
	lower := strings.ToLower(combined)

	score := lengthScore(combined) +
		turnScore(req.Messages) +
		keywordScore(lower) +
		structureScore(combined)

	if score > 1.0 {
		score = 1.0
	}

	return Analysis{
		Score:    score,
		TaskType: classifyTask(lower),
	}
}

// classifyTask returns the most specific matching task type.
// Priority: qa > writing > planning > general.
func classifyTask(lower string) TaskType {
	if matchesAny(lower, qaKeywords) {
		return TaskQA
	}
	if matchesAny(lower, writingKeywords) {
		return TaskWriting
	}
	if matchesAny(lower, planningKeywords) {
		return TaskPlanning
	}
	return TaskGeneral
}

func matchesAny(lower string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func lengthScore(combined string) float64 {
	chars := float64(len(combined))
	if chars >= 3000 {
		return 0.25
	}
	return chars / 3000 * 0.25
}

func turnScore(messages []api.Message) float64 {
	turns := float64(len(messages))
	if turns >= 10 {
		return 0.15
	}
	return turns / 10 * 0.15
}

func keywordScore(lower string) float64 {
	hits := 0
	for _, kw := range complexKeywords {
		if strings.Contains(lower, kw) {
			hits++
		}
	}
	if hits >= 4 {
		return 0.35
	}
	return float64(hits) / 4 * 0.35
}

func structureScore(combined string) float64 {
	score := 0.0
	if strings.Count(combined, "```") >= 1 {
		score += 0.15
	}
	if strings.Contains(combined, "$$") || strings.Contains(combined, `\[`) {
		score += 0.10
	} else if looksLikeJSON(combined) {
		score += 0.05
	}
	if score > 0.25 {
		return 0.25
	}
	return score
}

func looksLikeJSON(s string) bool {
	opens := strings.Count(s, "{")
	closes := strings.Count(s, "}")
	return opens >= 3 && opens == closes
}

func combinedContent(messages []api.Message) string {
	var sb strings.Builder
	for _, m := range messages {
		sb.WriteString(m.Content)
		sb.WriteByte(' ')
	}
	return sb.String()
}
