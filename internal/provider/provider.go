// Defines the Provider interface implemented by all LLM backend adapters.
//
// Author: Justin Campbell
package provider

import (
	"context"

	"conductor/internal/api"
)

type Provider interface {
	Complete(ctx context.Context, req *api.ChatCompletionRequest) (*api.ChatCompletionResponse, error)
	Name() string
}
