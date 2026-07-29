// Tests for the flat k-NN index and the semantic Store integration.
//
// Author: Justin Campbell
package vec

import (
	"testing"

	"conductor/internal/api"
)

func makeReq(model, content string) *api.ChatCompletionRequest {
	return &api.ChatCompletionRequest{
		Model:    model,
		Messages: []api.Message{{Role: "user", Content: content}},
	}
}

func makeTestResp(content string) *api.ChatCompletionResponse {
	return &api.ChatCompletionResponse{
		Choices: []api.ChatCompletionChoice{
			{Message: api.Message{Role: "assistant", Content: content}},
		},
	}
}

func TestIndex_SearchHit(t *testing.T) {
	idx := &Index{}
	v := Vector{"go": 0.6, "runtime": 0.8}
	idx.Add("doc1", v)

	matches := idx.Search(v, 1, 0.99)
	if len(matches) == 0 {
		t.Fatal("expected a match for identical vector")
	}
	if matches[0].ID != "doc1" {
		t.Errorf("got ID %q, want doc1", matches[0].ID)
	}
	if matches[0].Score < 0.99 {
		t.Errorf("score %.4f, want >= 0.99", matches[0].Score)
	}
}

func TestIndex_SearchMiss(t *testing.T) {
	idx := &Index{}
	idx.Add("doc1", Vector{"go": 0.6, "runtime": 0.8})

	orthogonal := Vector{"python": 1.0}
	matches := idx.Search(orthogonal, 1, 0.5)
	if len(matches) != 0 {
		t.Errorf("expected no match, got %d", len(matches))
	}
}

func TestIndex_TopK(t *testing.T) {
	idx := &Index{}
	idx.Add("low", Vector{"a": 0.6, "b": 0.8})
	idx.Add("high", Vector{"a": 0.8, "b": 0.6})

	query := Vector{"a": 1.0}
	matches := idx.Search(query, 1, 0.0)
	if len(matches) != 1 {
		t.Fatalf("expected 1 result, got %d", len(matches))
	}
	if matches[0].ID != "high" {
		t.Errorf("expected top result to be 'high', got %q", matches[0].ID)
	}
}

func TestIndex_InsertSortedDescending(t *testing.T) {
	var results []Match
	results = insertSorted(results, Match{ID: "b", Score: 0.7}, 3)
	results = insertSorted(results, Match{ID: "a", Score: 0.9}, 3)
	results = insertSorted(results, Match{ID: "c", Score: 0.5}, 3)

	if results[0].ID != "a" || results[1].ID != "b" || results[2].ID != "c" {
		t.Errorf("wrong order: %v", results)
	}
}

func TestIndex_InsertSortedCapAtK(t *testing.T) {
	var results []Match
	for i := 0; i < 5; i++ {
		results = insertSorted(results, Match{Score: float64(i)}, 3)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	if results[0].Score != 4.0 {
		t.Errorf("expected top score 4.0, got %.1f", results[0].Score)
	}
}

func TestStore_SemanticMatch(t *testing.T) {
	s := NewStore()

	// Store a response about garbage collection with rich terminology.
	req1 := makeReq("gpt-4o-mini", "garbage collection algorithm memory runtime heap allocation")
	resp1 := makeTestResp("GC pauses stop-the-world briefly...")
	s.Add("key1", req1, resp1)

	s.Add("key2", makeReq("gpt-4o-mini", "tcp protocol network packets transmission latency"), makeTestResp("TCP is reliable..."))
	s.Add("key3", makeReq("gpt-4o-mini", "hash table bucket collision chaining open addressing"), makeTestResp("Hash tables use..."))

	// Query shares: garbage, collection, algorithm, memory, runtime — 5 out of 5 key1 terms.
	query := makeReq("gpt-4o-mini", "garbage collection algorithm memory runtime performance")
	got, score, ok := s.Search(query, 0.7)
	if !ok {
		t.Fatalf("expected semantic match above 0.7, none found")
	}
	if got.Choices[0].Message.Content != "GC pauses stop-the-world briefly..." {
		t.Errorf("wrong response returned: %q", got.Choices[0].Message.Content)
	}
	t.Logf("semantic match score: %.4f", score)
}

func TestStore_NoMatchBelowThreshold(t *testing.T) {
	s := NewStore()
	s.Add("key1", makeReq("gpt-4o-mini", "explain garbage collection in Go"), makeTestResp("GC..."))

	query := makeReq("gpt-4o-mini", "calculate the integral of x squared from zero to one")
	_, _, ok := s.Search(query, 0.85)
	if ok {
		t.Error("expected no match for unrelated query above 0.85 threshold")
	}
}

func TestStore_ColdCorpus(t *testing.T) {
	s := NewStore()
	query := makeReq("gpt-4o-mini", "anything at all")
	_, _, ok := s.Search(query, 0.5)
	if ok {
		t.Error("expected no match on cold corpus")
	}
}
