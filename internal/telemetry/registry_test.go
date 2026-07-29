package telemetry

import (
	"sync"
	"testing"
)

func TestRecord_ExactHit(t *testing.T) {
	r := NewRegistry()
	r.Record("alice", "gpt-4o-mini", "hit", 100)
	snap := r.Snapshot()
	if snap.Global.Requests != 1 {
		t.Errorf("global requests = %d, want 1", snap.Global.Requests)
	}
	if snap.Global.ExactHits != 1 {
		t.Errorf("global exact_hits = %d, want 1", snap.Global.ExactHits)
	}
	if snap.Callers["alice"].ExactHits != 1 {
		t.Errorf("alice exact_hits = %d, want 1", snap.Callers["alice"].ExactHits)
	}
}

func TestRecord_SemanticHit(t *testing.T) {
	r := NewRegistry()
	r.Record("bob", "gpt-4o-mini", "semantic-hit:0.85", 50)
	snap := r.Snapshot()
	if snap.Global.SemanticHits != 1 {
		t.Errorf("global semantic_hits = %d, want 1", snap.Global.SemanticHits)
	}
}

func TestRecord_Miss(t *testing.T) {
	r := NewRegistry()
	r.Record("carol", "claude-opus-5", "miss", 200)
	snap := r.Snapshot()
	if snap.Global.Misses != 1 {
		t.Errorf("global misses = %d, want 1", snap.Global.Misses)
	}
	if snap.Global.ComplexRoutes != 1 {
		t.Errorf("global complex_routes = %d, want 1", snap.Global.ComplexRoutes)
	}
}

func TestRecord_SimpleRoute(t *testing.T) {
	r := NewRegistry()
	r.Record("dave", "gpt-4o-mini", "miss", 30)
	snap := r.Snapshot()
	if snap.Global.SimpleRoutes != 1 {
		t.Errorf("global simple_routes = %d, want 1", snap.Global.SimpleRoutes)
	}
}

func TestRecord_AnonymousCaller(t *testing.T) {
	r := NewRegistry()
	r.Record("", "gpt-4o-mini", "hit", 10)
	snap := r.Snapshot()
	if _, ok := snap.Callers["_anonymous"]; !ok {
		t.Error("expected _anonymous key for empty caller ID")
	}
}

func TestRecord_TokenAccumulation(t *testing.T) {
	r := NewRegistry()
	r.Record("eve", "gpt-4o-mini", "miss", 100)
	r.Record("eve", "gpt-4o-mini", "miss", 200)
	snap := r.Snapshot()
	if snap.Callers["eve"].Tokens != 300 {
		t.Errorf("tokens = %d, want 300", snap.Callers["eve"].Tokens)
	}
}

func TestRecord_Concurrent(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Record("user", "gpt-4o-mini", "hit", 10)
		}()
	}
	wg.Wait()
	snap := r.Snapshot()
	if snap.Global.Requests != 100 {
		t.Errorf("global requests = %d, want 100", snap.Global.Requests)
	}
}
