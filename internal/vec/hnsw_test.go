package vec

import (
	"fmt"
	"sync"
	"testing"
)

// makeVec creates a sparse unit vector for testing.
func makeVec(terms ...string) Vector {
	v := make(Vector, len(terms))
	for _, t := range terms {
		v[t] = 1.0
	}
	return normalise(v)
}

func TestHNSW_SingleNode(t *testing.T) {
	h := NewHNSW()
	h.Add("a", makeVec("alpha", "bravo"))
	if h.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", h.Len())
	}
}

func TestHNSW_SearchFindsExactMatch(t *testing.T) {
	h := NewHNSW()
	v := makeVec("golang", "runtime", "garbage", "collection")
	h.Add("gc", v)

	matches := h.Search(v, 1, 0.99)
	if len(matches) == 0 {
		t.Fatal("expected match for identical vector, got none")
	}
	if matches[0].ID != "gc" {
		t.Errorf("got %q, want %q", matches[0].ID, "gc")
	}
}

func TestHNSW_SearchRespectsThreshold(t *testing.T) {
	h := NewHNSW()
	h.Add("a", makeVec("alpha", "bravo"))
	h.Add("b", makeVec("charlie", "delta"))

	// Search for something completely different — should return nothing above 0.9.
	matches := h.Search(makeVec("zulu", "yankee"), 5, 0.9)
	if len(matches) != 0 {
		t.Errorf("expected no match above 0.9 threshold, got %d", len(matches))
	}
}

func TestHNSW_NearestNeighbor(t *testing.T) {
	h := NewHNSW()
	// Two vectors: one shares 3 terms with query, one shares 0.
	h.Add("close", makeVec("alpha", "bravo", "charlie"))
	h.Add("far", makeVec("zulu", "yankee", "xray"))
	// Add a bunch of noise nodes so the graph is non-trivial.
	for i := 0; i < 20; i++ {
		h.Add(fmt.Sprintf("noise%d", i), makeVec(
			fmt.Sprintf("term%d", i),
			fmt.Sprintf("word%d", i),
		))
	}

	query := makeVec("alpha", "bravo", "charlie")
	matches := h.Search(query, 1, 0.5)
	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}
	if matches[0].ID != "close" {
		t.Errorf("top result = %q, want %q", matches[0].ID, "close")
	}
}

func TestHNSW_EmptyIndex(t *testing.T) {
	h := NewHNSW()
	matches := h.Search(makeVec("anything"), 5, 0.5)
	if len(matches) != 0 {
		t.Errorf("expected empty result from empty index, got %d", len(matches))
	}
}

func TestHNSW_EmptyVector(t *testing.T) {
	h := NewHNSW()
	h.Add("a", makeVec("alpha"))
	matches := h.Search(Vector{}, 5, 0.0)
	if len(matches) != 0 {
		t.Errorf("empty query vector should return no matches, got %d", len(matches))
	}
}

func TestHNSW_ConcurrentAddSearch(t *testing.T) {
	h := NewHNSW()
	var wg sync.WaitGroup

	// Writers
	for i := 0; i < 20; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.Add(fmt.Sprintf("node%d", i), makeVec(
				fmt.Sprintf("t%d", i),
				fmt.Sprintf("u%d", i),
			))
		}()
	}

	// Readers run concurrently with writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.Search(makeVec("t1", "u1"), 3, 0.0)
		}()
	}

	wg.Wait()
	if h.Len() != 20 {
		t.Errorf("Len() = %d, want 20", h.Len())
	}
}

// TestHNSW_IndexerInterface verifies HNSW satisfies the Indexer interface.
func TestHNSW_IndexerInterface(t *testing.T) {
	var _ Indexer = NewHNSW()
}

// BenchmarkHNSW_Add measures insertion throughput.
func BenchmarkHNSW_Add(b *testing.B) {
	h := NewHNSW()
	vecs := make([]Vector, b.N)
	for i := range vecs {
		vecs[i] = makeVec(fmt.Sprintf("term%d", i), fmt.Sprintf("word%d", i))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Add(fmt.Sprintf("n%d", i), vecs[i])
	}
}

// BenchmarkHNSW_Search measures search throughput on a 1000-node index.
func BenchmarkHNSW_Search(b *testing.B) {
	h := NewHNSW()
	for i := 0; i < 1000; i++ {
		h.Add(fmt.Sprintf("n%d", i), makeVec(
			fmt.Sprintf("term%d", i),
			fmt.Sprintf("word%d", i%50),
		))
	}
	query := makeVec("term42", "word42")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Search(query, 5, 0.0)
	}
}
