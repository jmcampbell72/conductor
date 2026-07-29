// L2-normalised term-frequency vectors and cosine similarity. IDF is
// intentionally omitted; stopword removal handles common-term downweighting
// without causing corpus drift as the cache grows.
//
// Author: Justin Campbell
package vec

import "math"

// Vector is a sparse, L2-normalised term-frequency weight map.
type Vector map[string]float64

// Cosine returns the dot product of two L2-normalised vectors, equal to cosine similarity.
func Cosine(a, b Vector) float64 {
	if len(a) > len(b) {
		a, b = b, a
	}
	var dot float64
	for t, wa := range a {
		if wb, ok := b[t]; ok {
			dot += wa * wb
		}
	}
	return dot
}

// Embedder tokenizes text and produces L2-normalised term-frequency vectors.
// IDF is intentionally omitted: stopword removal already handles common-term
// downweighting, and pure TF avoids corpus-drift (stored vectors becoming
// incompatible with query vectors as the cache grows).
// Phase 7 will replace this with an HNSW-backed neural embedder.
type Embedder struct{}

func NewEmbedder() *Embedder { return &Embedder{} }

// Add and Embed are identical — both return a normalised TF vector.
// Add exists as a separate method so the call site signals intent
// (this document is entering the index) and can be swapped for a
// stateful embedder in Phase 7 without changing the Store.
func (e *Embedder) Add(text string) Vector  { return embed(text) }
func (e *Embedder) Embed(text string) Vector { return embed(text) }

func embed(text string) Vector {
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return Vector{}
	}
	return normalise(termFreq(tokens))
}

func termFreq(tokens []string) Vector {
	counts := make(map[string]int, len(tokens))
	for _, t := range tokens {
		counts[t]++
	}
	total := float64(len(tokens))
	v := make(Vector, len(counts))
	for t, c := range counts {
		v[t] = float64(c) / total
	}
	return v
}

func normalise(v Vector) Vector {
	var norm float64
	for _, w := range v {
		norm += w * w
	}
	if norm == 0 {
		return v
	}
	norm = math.Sqrt(norm)
	for t := range v {
		v[t] /= norm
	}
	return v
}
