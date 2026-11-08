//go:build !aego_runtime

package docs

import (
	"errors"
	"strconv"
)

const ExportVersion = 2

type Node struct {
	ID       string `json:"id"`
	Children []Node `json:"children,omitempty"`
}

type GroupNav struct {
	Group
	Nav []Node `json:"nav"`
}

type Export struct {
	Version  int               `json:"version"`
	Title    string            `json:"title"`
	Entries  []Entry           `json:"entries"`
	Nav      []Node            `json:"nav"`
	Groups   []GroupNav        `json:"groups"`
	Problems []Problem         `json:"problems"`
	Aliases  map[string]string `json:"aliases,omitempty"`
}

func (k Kind) MarshalText() ([]byte, error) {
	return []byte(k.String()), nil
}

func (k *Kind) UnmarshalText(b []byte) error {
	for i, n := range kindNames {
		if n == string(b) {
			*k = Kind(i)
			return nil
		}
	}
	return errors.New("unknown entry kind " + strconv.Quote(string(b)))
}

func (r *Registry) Export(title string) Export {
	exp := Export{Version: ExportVersion, Title: title, Entries: r.All(), Problems: r.Check(false), Aliases: r.Aliases()}
	for i := range exp.Entries {
		hideChecks(&exp.Entries[i])
	}
	for _, e := range r.Roots() {
		exp.Nav = append(exp.Nav, r.node(e.ID, 0))
	}
	exp.Groups = r.GroupNavigation()
	return exp
}

// GroupNavigation builds the same grouped tree as Export without exporting
// every entry or running documentation diagnostics. The editor's reference
// panel only needs this tree; full exports still perform all checks.
func (r *Registry) GroupNavigation() []GroupNav {
	var out []GroupNav
	for _, g := range r.Groups() {
		gn := GroupNav{Group: g}
		for _, e := range r.GroupRoots(g.ID) {
			gn.Nav = append(gn.Nav, r.node(e.ID, 0))
		}
		if len(gn.Nav) > 0 {
			out = append(out, gn)
		}
	}
	return out
}

func (r *Registry) node(id string, depth int) Node {
	n := Node{ID: id}
	if depth > 32 {
		return n
	}
	for _, c := range r.Children(id) {
		n.Children = append(n.Children, r.node(c.ID, depth+1))
	}
	return n
}

// hideChecks removes the check scenes from an exported entry and its translations, copying the map it changes.
func hideChecks(e *Entry) {
	e.Body = HideChecks(e.Body)
	if len(e.Translations) == 0 {
		return
	}
	ts := make(map[string]Translation, len(e.Translations))
	for l, t := range e.Translations {
		t.Body = HideChecks(t.Body)
		ts[l] = t
	}
	e.Translations = ts
}
