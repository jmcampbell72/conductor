// Applies per-route output controls to requests before provider dispatch.
// Enforces max_tokens caps, response_format type, and concise-response
// instruction injection. The original request is never mutated.
//
// Author: Justin Campbell
package output

import (
	"conductor/internal/api"
	"conductor/internal/config"
)

const conciseInstruction = "Be concise. Respond in as few words as possible while remaining accurate and complete."

// Enforcer applies per-route output controls to a request before it is sent
// to a provider. It must be called after model selection and before the cache
// key is computed, so the key reflects any injected messages or format changes.
type Enforcer struct{}

func New() *Enforcer { return &Enforcer{} }

// Apply returns a copy of req with the route's output controls applied.
// The original is never mutated.
func (e *Enforcer) Apply(req *api.ChatCompletionRequest, route config.RouteConfig) *api.ChatCompletionRequest {
	out := *req // shallow copy — safe because we replace slice/pointer fields, never mutate in place

	if route.MaxTokens > 0 && (out.MaxTokens == 0 || out.MaxTokens > route.MaxTokens) {
		out.MaxTokens = route.MaxTokens
	}

	if route.ResponseFormatType != "" && out.ResponseFormat == nil {
		out.ResponseFormat = &api.ResponseFormat{Type: route.ResponseFormatType}
	}

	if route.ConciseResponse {
		out.Messages = injectConcise(out.Messages)
	}

	return &out
}

// injectConcise inserts the concise-response instruction as a system message
// immediately after the caller's last system message. If there are no system
// messages, the instruction is prepended so it comes before any user turn.
// Inserting last among system messages lets it override a less-specific
// caller system prompt.
func injectConcise(msgs []api.Message) []api.Message {
	lastSys := -1
	for i, m := range msgs {
		if m.Role == "system" {
			lastSys = i
		}
	}

	conciseMsg := api.Message{Role: "system", Content: conciseInstruction}
	insertAt := lastSys + 1 // 0 when no system messages — prepends

	result := make([]api.Message, 0, len(msgs)+1)
	result = append(result, msgs[:insertAt]...)
	result = append(result, conciseMsg)
	result = append(result, msgs[insertAt:]...)
	return result
}
