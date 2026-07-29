package vec

import (
	"math"
	"math/rand"
	"sort"
	"sync"
)

const (
	hnswDefaultM              = 16
	hnswDefaultEfConstruction = 200
	hnswDefaultEf             = 50
	hnswMaxLevel              = 32
)

type hnswItem struct {
	id   string
	dist float64 // 1 - cosine(a,b); smaller = more similar
}

type hnswNode struct {
	id      string
	vec     Vector
	level   int
	friends [][]string // friends[layer] → neighbor IDs at that layer
}

// HNSW is a Hierarchical Navigable Small World graph index.
// It provides approximate O(log n) nearest-neighbor search, replacing the
// flat O(n) scan from Phase 4.
//
// Thread safety: concurrent Search calls are allowed; Add is serialised
// against both Add and Search via a single RWMutex (read = search, write = add).
type HNSW struct {
	mu             sync.RWMutex
	nodes          map[string]*hnswNode
	entryPoint     string
	maxLevel       int
	M              int     // max connections per layer (except layer 0)
	mMax0          int     // max connections at layer 0 = M*2
	efConstruction int     // beam width during construction
	ef             int     // beam width during search
	mL             float64 // 1/ln(M) — normalisation factor for level assignment
}

// NewHNSW builds an empty HNSW index with default parameters.
func NewHNSW() *HNSW {
	M := hnswDefaultM
	return &HNSW{
		nodes:          make(map[string]*hnswNode),
		maxLevel:       -1,
		M:              M,
		mMax0:          M * 2,
		efConstruction: hnswDefaultEfConstruction,
		ef:             hnswDefaultEf,
		mL:             1.0 / math.Log(float64(M)),
	}
}

// Add inserts a vector into the HNSW graph under the given id.
func (h *HNSW) Add(id string, vec Vector) {
	h.mu.Lock()
	defer h.mu.Unlock()

	level := h.randomLevel()
	node := &hnswNode{
		id:      id,
		vec:     vec,
		level:   level,
		friends: make([][]string, level+1),
	}
	for i := range node.friends {
		node.friends[i] = make([]string, 0, h.M)
	}
	h.nodes[id] = node

	// First node becomes the entry point.
	if h.entryPoint == "" {
		h.entryPoint = id
		h.maxLevel = level
		return
	}

	ep := []string{h.entryPoint}

	// Phase 1: greedy descent from maxLevel to level+1 to find a good entry point.
	for lc := h.maxLevel; lc > level; lc-- {
		ep = h.searchLayer(vec, ep, 1, lc)
	}

	// Phase 2: beam search + connect at each layer from min(level,maxLevel) down to 0.
	for lc := intMin(level, h.maxLevel); lc >= 0; lc-- {
		maxM := h.M
		if lc == 0 {
			maxM = h.mMax0
		}
		candidates := h.searchLayer(vec, ep, h.efConstruction, lc)

		selected := candidates
		if len(selected) > maxM {
			selected = selected[:maxM]
		}
		node.friends[lc] = append(node.friends[lc], selected...)

		for _, nid := range selected {
			neighbor := h.nodes[nid]
			if lc < len(neighbor.friends) {
				neighbor.friends[lc] = append(neighbor.friends[lc], id)
				if len(neighbor.friends[lc]) > maxM {
					neighbor.friends[lc] = h.pruneConnections(neighbor.vec, neighbor.friends[lc], maxM)
				}
			}
		}
		ep = candidates
	}

	if level > h.maxLevel {
		h.maxLevel = level
		h.entryPoint = id
	}
}

// Search returns the top-k matches with cosine similarity ≥ threshold.
func (h *HNSW) Search(q Vector, k int, threshold float64) []Match {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.entryPoint == "" || len(q) == 0 {
		return nil
	}

	ep := []string{h.entryPoint}
	for lc := h.maxLevel; lc > 0; lc-- {
		ep = h.searchLayer(q, ep, 1, lc)
	}
	candidates := h.searchLayer(q, ep, h.ef, 0)

	results := make([]Match, 0, k)
	for _, id := range candidates {
		if score := Cosine(q, h.nodes[id].vec); score >= threshold {
			results = insertSorted(results, Match{ID: id, Score: score}, k)
		}
	}
	return results
}

// Len returns the number of indexed vectors.
func (h *HNSW) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.nodes)
}

// searchLayer returns up to ef nearest node IDs to q at layer lc, starting from ep.
// Results are ordered ascending by distance (best match first).
func (h *HNSW) searchLayer(q Vector, ep []string, ef, lc int) []string {
	visited := make(map[string]bool, ef*2)
	for _, e := range ep {
		visited[e] = true
	}

	candidates := make([]hnswItem, 0, ef)
	result := make([]hnswItem, 0, ef)

	for _, e := range ep {
		d := 1.0 - Cosine(q, h.nodes[e].vec)
		candidates = hnswInsert(candidates, hnswItem{e, d})
		result = hnswInsert(result, hnswItem{e, d})
	}

	for len(candidates) > 0 {
		c := candidates[0]
		candidates = candidates[1:]

		worstDist := math.MaxFloat64
		if len(result) >= ef {
			worstDist = result[len(result)-1].dist
		}
		if c.dist > worstDist {
			break
		}

		node := h.nodes[c.id]
		if lc < len(node.friends) {
			for _, nid := range node.friends[lc] {
				if visited[nid] {
					continue
				}
				visited[nid] = true

				d := 1.0 - Cosine(q, h.nodes[nid].vec)
				worstDist = math.MaxFloat64
				if len(result) >= ef {
					worstDist = result[len(result)-1].dist
				}
				if d < worstDist || len(result) < ef {
					candidates = hnswInsert(candidates, hnswItem{nid, d})
					result = hnswInsert(result, hnswItem{nid, d})
					if len(result) > ef {
						result = result[:ef]
					}
				}
			}
		}
	}

	ids := make([]string, len(result))
	for i, r := range result {
		ids[i] = r.id
	}
	return ids
}

// pruneConnections keeps the maxM closest connections to anchor.
func (h *HNSW) pruneConnections(anchor Vector, friends []string, maxM int) []string {
	items := make([]hnswItem, len(friends))
	for i, fid := range friends {
		items[i] = hnswItem{fid, 1.0 - Cosine(anchor, h.nodes[fid].vec)}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].dist < items[j].dist })
	if len(items) > maxM {
		items = items[:maxM]
	}
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.id
	}
	return out
}

// randomLevel assigns a node to a random maximum layer following HNSW's
// geometric distribution: P(level ≥ l) = (1/M)^l.
func (h *HNSW) randomLevel() int {
	r := rand.Float64()
	if r == 0 {
		return 0
	}
	l := int(-math.Log(r) * h.mL)
	if l > hnswMaxLevel {
		l = hnswMaxLevel
	}
	return l
}

// hnswInsert inserts item into s maintaining ascending-dist order.
func hnswInsert(s []hnswItem, item hnswItem) []hnswItem {
	i := sort.Search(len(s), func(j int) bool { return s[j].dist > item.dist })
	s = append(s, hnswItem{})
	copy(s[i+1:], s[i:])
	s[i] = item
	return s
}

func intMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
