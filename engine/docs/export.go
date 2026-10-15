//go:build !aego_runtime

package docs

import (
	"errors"
	"strconv"
)

const ExportVersion = 1

type Node struct {
	ID       string `json:"id"`
	Children []Node `json:"children,omitempty"`
}

type Export struct {
	Version  int       `json:"version"`
	Title    string    `json:"title"`
	Entries  []Entry   `json:"entries"`
	Nav      []Node    `json:"nav"`
	Problems []Problem `json:"problems"`
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
	exp := Export{Version: ExportVersion, Title: title, Entries: r.All(), Problems: r.Check(false)}
	for _, e := range r.Roots() {
		exp.Nav = append(exp.Nav, r.node(e.ID, 0))
	}
	return exp
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
