// OpenAI-compatible provider supporting both the public OpenAI API and private
// hosted endpoints. NewOpenAI targets api.openai.com; NewHosted accepts any
// base URL for private inference endpoints.
//
// Author: Justin Campbell
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"conductor/internal/api"
)

const openAIBase = "https://api.openai.com/v1"

// OpenAICompatible calls any OpenAI-compatible chat completions endpoint.
type OpenAICompatible struct {
	name    string
	baseURL string
	key     string
	client  *http.Client
}

func NewOpenAI(key string) *OpenAICompatible {
	return NewHosted("openai", openAIBase, key)
}

// NewHosted creates a provider for a private OpenAI-compatible endpoint.
func NewHosted(name, baseURL, key string) *OpenAICompatible {
	return &OpenAICompatible{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		key:     key,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OpenAICompatible) Name() string { return p.name }

func (p *OpenAICompatible) Complete(ctx context.Context, req *api.ChatCompletionRequest) (*api.ChatCompletionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.key)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp api.ErrorResponse
		if jsonErr := json.NewDecoder(resp.Body).Decode(&errResp); jsonErr != nil {
			return nil, fmt.Errorf("%s: HTTP %d", p.name, resp.StatusCode)
		}
		return nil, fmt.Errorf("%s: %s", p.name, errResp.Error.Message)
	}

	var result api.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
