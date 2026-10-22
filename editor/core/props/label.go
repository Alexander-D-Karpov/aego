package props

import (
	"strings"
	"sync"

	"aego/scene"
	"aego/schema"
)

const nodeFallbackIcon = "cat.node"

var ranks struct {
	sync.Mutex
	reg  *schema.Registry
	rank map[schema.TypeID]int
}

func rankOf(reg *schema.Registry) map[schema.TypeID]int {
	ranks.Lock()
	defer ranks.Unlock()
	if ranks.reg != reg {
		ranks.reg, ranks.rank = reg, map[schema.TypeID]int{}
		for i, td := range reg.Components() {
			ranks.rank[td.ID] = i
		}
	}
	return ranks.rank
}

// NodeIconOf picks the icon of the visible component with the highest NodeIcon, the earliest registered on a tie; empty is true when the node has no visible components.
func NodeIconOf(s *scene.Scene, h scene.NodeHandle) (icon string, empty bool) {
	reg := s.Registry()
	rank := rankOf(reg)
	prio, best, any := 0, 0, false
	s.ComponentsOf(h, func(t schema.TypeID) {
		td, ok := reg.ByID(t)
		if !ok || td.Hidden {
			return
		}
		any = true
		r := rank[t]
		if td.NodeIcon > 0 && td.Icon != "" && (td.NodeIcon > prio || td.NodeIcon == prio && r < best) {
			icon, prio, best = td.Icon, td.NodeIcon, r
		}
	})
	if icon == "" && any {
		icon = nodeFallbackIcon
	}
	return icon, !any
}

func NodeIcon(s *scene.Scene, h scene.NodeHandle) string {
	if icon, _ := NodeIconOf(s, h); icon != "" {
		return icon
	}
	return nodeFallbackIcon
}

// NodeLabel is how the editor names a node: its icon, its name (with the parent's when disambiguate) and the path as the tooltip.
func NodeLabel(s *scene.Scene, h scene.NodeHandle, disambiguate bool) (icon, text, tip string) {
	if h.IsZero() || !s.IsAlive(h) {
		return "", "", ""
	}
	text = s.Name(h)
	if p := s.Parent(h); disambiguate && !p.IsZero() && p != s.Root() {
		text = s.Name(p) + " › " + text
	}
	return NodeIcon(s, h), text, strings.TrimPrefix(s.Path(h), "/")
}

// Ambiguous reports the names that more than one node in list shares.
func Ambiguous(s *scene.Scene, list []scene.NodeHandle) map[string]bool {
	seen := make(map[string]int, len(list))
	for _, h := range list {
		seen[s.Name(h)]++
	}
	out := map[string]bool{}
	for n, c := range seen {
		if c > 1 {
			out[n] = true
		}
	}
	return out
}
