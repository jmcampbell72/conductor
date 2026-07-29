package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"conductor/internal/api"
)

const openAIBase = "https://api.openai.com/v1"

type OpenAI struct {
	key    string
	client *http.Client
}

func NewOpenAI(key string) *OpenAI {
	return &OpenAI{
		key:    key,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OpenAI) Name() string { return "openai" }

func (p *OpenAI) Complete(ctx context.Context, req *api.ChatCompletionRequest) (*api.ChatCompletionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIBase+"/chat/completions", bytes.NewReader(body))
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
			return nil, fmt.Errorf("openai: HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("openai: %s", errResp.Error.Message)
	}

	var result api.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
