package vec

import "sync"

// Match is a search result from the flat k-NN index.
type Match struct {
	ID    string
	Score float64
}

type indexEntry struct {
	id  string
	vec Vector
}

// Index is a flat k-NN index that scans all entries linearly.
// It is correct for small corpora; Phase 7 replaces it with HNSW.
type Index struct {
	mu      sync.RWMutex
	entries []indexEntry
}

// Add appends a vector to the index.
func (idx *Index) Add(id string, vec Vector) {
	idx.mu.Lock()
	idx.entries = append(idx.entries, indexEntry{id: id, vec: vec})
	idx.mu.Unlock()
}

// Search returns the top-k matches with cosine similarity ≥ threshold,
// sorted by descending score.
func (idx *Index) Search(vec Vector, k int, threshold float64) []Match {
	idx.mu.RLock()
	entries := idx.entries
	idx.mu.RUnlock()

	results := make([]Match, 0, k)
	for _, e := range entries {
		if s := Cosine(vec, e.vec); s >= threshold {
			results = insertSorted(results, Match{ID: e.id, Score: s}, k)
		}
	}
	return results
}

// Len returns the number of indexed vectors.
func (idx *Index) Len() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.entries)
}

// insertSorted inserts m into results maintaining descending Score order, capped at k.
func insertSorted(results []Match, m Match, k int) []Match {
	i := len(results)
	for i > 0 && results[i-1].Score < m.Score {
		i--
	}
	results = append(results, Match{})
	copy(results[i+1:], results[i:])
	results[i] = m
	if len(results) > k {
		results = results[:k]
	}
	return results
}
