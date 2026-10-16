//go:build !aego_runtime

package docs

import "strings"

func (r *Registry) Compose(id string) string {
	i, ok := r.byID[id]
	if !ok {
		return ""
	}
	var b strings.Builder
	r.compose(&b, i, 0)
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func (r *Registry) compose(b *strings.Builder, i, depth int) {
	e := &r.entries[i]
	body := e.Body
	if !titled(body) {
		body = "# " + e.Title + "\n\n" + body
	}
	b.WriteString(demote(body, depth))
	b.WriteString("\n\n")
	for _, c := range r.children[e.ID] {
		if r.entries[c].Kind == Page {
			r.compose(b, c, depth+1)
		}
	}
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

func demote(md string, depth int) string {
	if depth == 0 {
		return md
	}
	lines := strings.Split(md, "\n")
	fence := false
	for i, l := range lines {
		if isFence(l) {
			fence = !fence
			continue
		}
		if fence || !strings.HasPrefix(l, "#") {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, "#"))
		if n < len(l) && l[n] == ' ' {
			lines[i] = strings.Repeat("#", min(depth, 6-n)) + l
		}
	}
	return strings.Join(lines, "\n")
}
