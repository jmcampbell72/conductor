// Token count estimation utilities using the ~4 characters per token rule of
// thumb for English text.
//
// Author: Justin Campbell
package trim

import "conductor/internal/api"

// EstimateTokens returns an approximate token count for a string.
// Uses the ~4 characters per token rule of thumb for English text.
func EstimateTokens(text string) int {
	return (len(text) + 3) / 4
}

// EstimateMessage returns an approximate token count for a single message,
// including the per-message overhead for role and framing bytes.
func EstimateMessage(m api.Message) int {
	return EstimateTokens(m.Content) + 4
}

// EstimateRequest returns an approximate token count for the full request.
func EstimateRequest(req *api.ChatCompletionRequest) int {
	total := 3 // baseline framing
	for _, m := range req.Messages {
		total += EstimateMessage(m)
	}
	return total
}
