package site

import (
	"encoding/json"
	"os"
	"strings"
)

const (
	KindPage      = "page"
	KindModule    = "module"
	KindFunction  = "function"
	KindType      = "type"
	KindField     = "field"
	KindMethod    = "method"
	KindSignal    = "signal"
	KindCode      = "code"
	KindConst     = "const"
	KindEnum      = "enum"
	ScriptsPrefix = "scripts/"
)

type Param struct {
	Name string
	Type string
}

type Location struct {
	Path   string
	Line   int
	Stub   string
	Symbol string
	Source string
}

type Snippet struct {
	Lang string
	Code string
	Run  bool
	Line int
}

type Entry struct {
	ID        string
	Parent    string
	Kind      string
	Owner     string
	Group     string
	Title     string
	Summary   string
	Body      string
	Signature string
	Params    []Param
	Example   string
	Location  Location
	Links     []string
	Keys      []string
	Requires  []string
	Snippets  []Snippet
	Order     int
	Field     *FieldInfo
	Component *ComponentInfo
	Related   []string
}

type FieldInfo struct {
	Type    string
	Default string
	Range   string
	Notes   []string
}

type ComponentInfo struct {
	Category string
	Module   string
	Space    string
	Unique   bool
	Requires []string
	Create   string
	Events   []string
}

type Node struct {
	ID       string `json:"id"`
	Children []Node `json:"children,omitempty"`
}

type Group struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Section string `json:"section"`
	Order   int    `json:"order"`
}

type GroupNav struct {
	Group
	Nav []Node `json:"nav"`
}

type Problem struct {
	ID      string
	Error   bool
	Message string
}

type Export struct {
	Version  int        `json:"version"`
	Title    string     `json:"title"`
	Entries  []Entry    `json:"entries"`
	Nav      []Node     `json:"nav"`
	Groups   []GroupNav `json:"groups"`
	Problems []Problem  `json:"problems"`
}

func Load(path string) (*Export, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var exp Export
	if err := json.Unmarshal(data, &exp); err != nil {
		return nil, err
	}
	if exp.Title == "" {
		exp.Title = "Aego"
	}
	return &exp, nil
}

func titled(md string) bool {
	for _, l := range strings.Split(md, "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		return strings.HasPrefix(l, "# ")
	}
	return false
}

func stripTitle(md string) string {
	lines := strings.Split(md, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if strings.HasPrefix(l, "# ") {
			return strings.TrimLeft(strings.Join(lines[i+1:], "\n"), "\n")
		}
		break
	}
	return md
}

func cleanPath(id string) string {
	var b strings.Builder
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '_', c == '-', c == '/':
			b.WriteRune(c)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "/")
}
