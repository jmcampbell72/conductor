package vec

import (
	"strings"
	"sync"

	"conductor/internal/api"
)

// Indexer is the interface satisfied by both the flat Index (Phase 4) and HNSW (Phase 7).
type Indexer interface {
	Add(id string, vec Vector)
	Search(vec Vector, k int, threshold float64) []Match
	Len() int
}

// Store is a semantic response cache: it embeds requests with TF-IDF and
// retrieves the most similar stored response above a cosine-similarity threshold.
type Store struct {
	embedder  *Embedder
	index     Indexer
	mu        sync.RWMutex
	responses map[string]*api.ChatCompletionResponse
}

// NewStore returns a Store backed by an HNSW index.
func NewStore() *Store {
	return &Store{
		embedder:  NewEmbedder(),
		index:     NewHNSW(),
		responses: make(map[string]*api.ChatCompletionResponse),
	}
}

// NewStoreWithIndex returns a Store backed by the provided Indexer.
// Used in tests to swap in the flat Index for deterministic behaviour.
func NewStoreWithIndex(idx Indexer) *Store {
	return &Store{
		embedder:  NewEmbedder(),
		index:     idx,
		responses: make(map[string]*api.ChatCompletionResponse),
	}
}

// Add indexes a request+response pair under id (the exact cache key).
// Calling Add updates the TF-IDF corpus so future Embed calls reflect this document.
func (s *Store) Add(id string, req *api.ChatCompletionRequest, resp *api.ChatCompletionResponse) {
	text := requestText(req)
	vec := s.embedder.Add(text)
	s.index.Add(id, vec)

	s.mu.Lock()
	s.responses[id] = resp
	s.mu.Unlock()
}

// Search returns the most similar cached response with cosine similarity ≥ threshold.
// Returns (nil, 0, false) when the corpus is cold or no match exceeds the threshold.
func (s *Store) Search(req *api.ChatCompletionRequest, threshold float64) (*api.ChatCompletionResponse, float64, bool) {
	text := requestText(req)
	vec := s.embedder.Embed(text)
	if len(vec) == 0 {
		return nil, 0, false
	}

	matches := s.index.Search(vec, 1, threshold)
	if len(matches) == 0 {
		return nil, 0, false
	}

	best := matches[0]
	s.mu.RLock()
	resp, ok := s.responses[best.ID]
	s.mu.RUnlock()
	if !ok {
		return nil, 0, false
	}
	return resp, best.Score, true
}

// Len returns the number of indexed responses.
func (s *Store) Len() int { return s.index.Len() }

func requestText(req *api.ChatCompletionRequest) string {
	var b strings.Builder
	for _, m := range req.Messages {
		b.WriteString(m.Content)
		b.WriteByte(' ')
	}
	return b.String()
}
