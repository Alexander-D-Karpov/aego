//go:build !aego_runtime

package docs

import (
	"sort"
	"strings"
)

func lastName(id string) string {
	return id[strings.LastIndexAny(id, "./")+1:]
}

func subseq(s, q string) int {
	first, last, j := -1, 0, 0
	for i := 0; i < len(s) && j < len(q); i++ {
		if s[i] == q[j] {
			if first < 0 {
				first = i
			}
			last = i
			j++
		}
	}
	if j < len(q) {
		return 0
	}
	return max(1, 200-(last-first+1-len(q))*5)
}

func match(s, q string) int {
	switch {
	case s == q:
		return 1000
	case strings.HasPrefix(s, q):
		return 800 - min(len(s)-len(q), 100)
	}
	if i := strings.Index(s, q); i >= 0 {
		return 500 - min(i, 100)
	}
	return subseq(s, q)
}

func score(e *Entry, q string) int {
	best := match(strings.ToLower(lastName(e.ID)), q)
	best = max(best, match(strings.ToLower(e.Title), q))
	for _, k := range e.Keys {
		best = max(best, match(strings.ToLower(k), q))
	}
	if s := match(strings.ToLower(e.ID), q); s > 0 {
		best = max(best, s/2)
	}
	return best
}

func (r *Registry) Search(q string, limit int) []Entry {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	type hit struct {
		i     int
		score int
	}
	var hits []hit
	for i := range r.entries {
		if s := score(&r.entries[i], q); s > 0 {
			hits = append(hits, hit{i, s})
		}
	}
	sort.Slice(hits, func(a, b int) bool {
		x, y := hits[a], hits[b]
		if x.score != y.score {
			return x.score > y.score
		}
		ex, ey := &r.entries[x.i], &r.entries[y.i]
		if ex.Kind != ey.Kind {
			return kindRank[ex.Kind] < kindRank[ey.Kind]
		}
		if len(ex.ID) != len(ey.ID) {
			return len(ex.ID) < len(ey.ID)
		}
		return ex.ID < ey.ID
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]Entry, len(hits))
	for k, h := range hits {
		out[k] = r.entries[h.i]
	}
	return out
}
