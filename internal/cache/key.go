package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"conductor/internal/api"
)

// normalised is the subset of a request that determines cache identity.
// Non-deterministic fields (user metadata, client timestamps) are excluded.
type normalised struct {
	Model          string              `json:"m"`
	Messages       []api.Message       `json:"ms"`
	MaxTokens      int                 `json:"mt,omitempty"`
	Temperature    *float64            `json:"t,omitempty"`
	ResponseFormat *api.ResponseFormat `json:"rf,omitempty"`
}

// Key returns a hex-encoded SHA-256 hash of the normalised request.
// The key is computed after trimming and model selection so both the
// trimmed content and the routed model are part of the identity.
func Key(req *api.ChatCompletionRequest) string {
	n := normalised{
		Model:          req.Model,
		Messages:       req.Messages,
		MaxTokens:      req.MaxTokens,
		Temperature:    req.Temperature,
		ResponseFormat: req.ResponseFormat,
	}
	data, _ := json.Marshal(n)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
