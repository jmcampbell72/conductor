package router

import (
	"strings"

	"conductor/internal/api"
)

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

// Analyzer scores requests on a 0.0–1.0 scale using four heuristic signals.
type Analyzer struct{}

func NewAnalyzer() *Analyzer { return &Analyzer{} }

// Score returns a complexity score in [0.0, 1.0].
// Four signals, with maximum contributions that sum to 1.0:
//
//	Length      0.25  saturates at 5000 characters
//	Turn count  0.15  saturates at 10 messages
//	Keywords    0.35  saturates at 4 matched keywords
//	Structure   0.25  code fences (0.15) + equations/JSON (up to 0.10)
func (a *Analyzer) Score(req *api.ChatCompletionRequest) float64 {
	combined := combinedContent(req.Messages)
	score := lengthScore(combined) +
		turnScore(req.Messages) +
		keywordScore(strings.ToLower(combined)) +
		structureScore(combined)

	if score > 1.0 {
		return 1.0
	}
	return score
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
