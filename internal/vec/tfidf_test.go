// Tests for the TF-IDF embedder: tokenization, L2 normalisation, cosine
// similarity, and Add vs Embed consistency.
//
// Author: Justin Campbell
package vec

import (
	"math"
	"testing"
)

func TestTokenize_Basic(t *testing.T) {
	tokens := tokenize("How do I implement a binary search tree?")
	want := map[string]bool{"implement": true, "binary": true, "search": true, "tree": true}
	for _, tok := range tokens {
		want[tok] = false // mark as seen
	}
	for tok, unseen := range want {
		if unseen {
			t.Errorf("expected token %q not found", tok)
		}
	}
}

func TestTokenize_StopwordsRemoved(t *testing.T) {
	tokens := tokenize("how do you get this done for all")
	for _, tok := range tokens {
		if stopwords[tok] {
			t.Errorf("stopword %q should have been removed", tok)
		}
	}
}

func TestTokenize_ShortTokensRemoved(t *testing.T) {
	tokens := tokenize("go is ok")
	// "go"=2 chars, "is"=stopword, "ok"=2 chars — all removed
	if len(tokens) != 0 {
		t.Errorf("expected empty result, got %v", tokens)
	}
}

func TestCosine_Identical(t *testing.T) {
	v := Vector{"alpha": 0.6, "bravo": 0.8}
	got := Cosine(v, v)
	if math.Abs(got-1.0) > 1e-9 {
		t.Errorf("cosine(v,v)=%.4f, want 1.0", got)
	}
}

func TestCosine_Orthogonal(t *testing.T) {
	a := Vector{"x": 1.0}
	b := Vector{"y": 1.0}
	if got := Cosine(a, b); got != 0 {
		t.Errorf("cosine of orthogonal vectors = %.4f, want 0", got)
	}
}

func TestEmbedder_EmptyText(t *testing.T) {
	e := NewEmbedder()
	v := e.Embed("")
	if len(v) != 0 {
		t.Errorf("expected empty vector for empty text, got %d terms", len(v))
	}
}

func TestEmbedder_AddEqualsEmbed(t *testing.T) {
	e := NewEmbedder()
	text := "explain garbage collection runtime memory management"
	va := e.Add(text)
	vb := e.Embed(text)
	for k, wa := range va {
		if wb, ok := vb[k]; !ok || math.Abs(wa-wb) > 1e-12 {
			t.Errorf("Add and Embed differ for term %q: %.6f vs %.6f", k, wa, wb)
		}
	}
}

func TestEmbedder_ProducesNormalisedVector(t *testing.T) {
	e := NewEmbedder()
	v := e.Add("explain garbage collection algorithm runtime memory")

	var norm float64
	for _, w := range v {
		norm += w * w
	}
	norm = math.Sqrt(norm)
	if math.Abs(norm-1.0) > 1e-9 {
		t.Errorf("vector L2 norm = %.6f, want 1.0", norm)
	}
}

func TestEmbedder_SimilarTextsHighSimilarity(t *testing.T) {
	e := NewEmbedder()
	// Both texts share garbage, collection, algorithm, memory, runtime
	v1 := e.Add("garbage collection algorithm memory runtime programming")
	v2 := e.Embed("garbage collection algorithm memory runtime performance")
	score := Cosine(v1, v2)
	if score < 0.7 {
		t.Errorf("similar texts cosine = %.4f, want >= 0.7", score)
	}
}

func TestEmbedder_DifferentTextsLowSimilarity(t *testing.T) {
	e := NewEmbedder()
	v1 := e.Embed("garbage collection algorithm memory runtime")
	v2 := e.Embed("integral calculus derivative trigonometry equation")
	score := Cosine(v1, v2)
	if score > 0.2 {
		t.Errorf("unrelated texts cosine = %.4f, want < 0.2", score)
	}
}
