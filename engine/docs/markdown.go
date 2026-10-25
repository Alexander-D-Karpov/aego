//go:build !aego_runtime

package docs

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type record struct {
	id         string
	title      string
	parent     string
	summary    string
	group      string
	groupTitle string
	groupIcon  string
	groupOrder int
	icon       string
	editorOnly bool
	order      int
	ordered    bool
	keys       []string
	requires   []string
	try        string
	trySelect  string
	code       string
	example    string
	scope      string
	severity   string
	body       string
	line       int
	styleAllow []string
}

func isFence(l string) bool {
	return strings.HasPrefix(strings.TrimSpace(l), "```")
}

func isKey(l string) bool {
	k, _, ok := strings.Cut(l, ":")
	if !ok || k == "" || k[0] < 'a' || k[0] > 'z' {
		return false
	}
	for _, c := range []byte(k) {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}

func isRule(l string) bool {
	return strings.TrimRight(l, " \t") == "---"
}

func opens(lines []string, i int) bool {
	return isRule(lines[i]) && i+1 < len(lines) && isKey(lines[i+1])
}

func parseRecords(src string) ([]record, error) {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	var out []record
	var body []string
	fence := false
	flush := func() {
		if len(out) > 0 {
			out[len(out)-1].body = strings.Trim(strings.Join(body, "\n"), "\n")
		}
		body = body[:0]
	}
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if !fence && opens(lines, i) {
			flush()
			rec := record{line: i + 1}
			closed := false
			for i++; i < len(lines); i++ {
				if isRule(lines[i]) {
					closed = true
					break
				}
				if strings.TrimSpace(lines[i]) == "" {
					continue
				}
				if !isKey(lines[i]) {
					return nil, fmt.Errorf("line %d: front matter needs key: value", i+1)
				}
				k, v, _ := strings.Cut(lines[i], ":")
				if err := rec.set(k, strings.TrimSpace(v)); err != nil {
					return nil, fmt.Errorf("line %d: %w", i+1, err)
				}
			}
			if !closed {
				return nil, fmt.Errorf("line %d: front matter is not closed with ---", rec.line)
			}
			if rec.id == "" {
				return nil, fmt.Errorf("line %d: front matter has no id", rec.line)
			}
			out = append(out, rec)
			continue
		}
		if isFence(l) {
			fence = !fence
		}
		if len(out) == 0 {
			if strings.TrimSpace(l) != "" {
				return nil, fmt.Errorf("line %d: text before the first front matter", i+1)
			}
			continue
		}
		body = append(body, l)
	}
	if fence {
		return nil, errors.New("code block is not closed")
	}
	flush()
	return out, nil
}

func (r *record) set(key, val string) error {
	var err error
	switch key {
	case "id":
		r.id, err = scalar(val)
	case "title":
		r.title, err = scalar(val)
	case "parent":
		r.parent, err = scalar(val)
	case "summary":
		r.summary, err = scalar(val)
	case "group":
		r.group, err = scalar(val)
	case "group_title":
		r.groupTitle, err = scalar(val)
	case "group_icon":
		r.groupIcon, err = scalar(val)
	case "group_order":
		var s string
		if s, err = scalar(val); err == nil {
			r.groupOrder, err = strconv.Atoi(s)
		}
	case "order":
		var s string
		if s, err = scalar(val); err == nil {
			r.order, err = strconv.Atoi(s)
			r.ordered = err == nil
		}
	case "keys":
		r.keys, err = list(val)
	case "requires":
		r.requires, err = list(val)
	case "try":
		r.try, err = scalar(val)
	case "try_select":
		r.trySelect, err = scalar(val)
	case "code":
		r.code, err = scalar(val)
		if r.id == "" {
			r.id = r.code
		}
	case "example":
		r.example, err = scalar(val)
	case "scope":
		r.scope, err = scalar(val)
	case "severity":
		r.severity, err = scalar(val)
	case "icon":
		r.icon, err = scalar(val)
	case "style_allow":
		r.styleAllow, err = list(val)
	case "editor_only":
		var v string
		if v, err = scalar(val); err == nil {
			r.editorOnly, err = strconv.ParseBool(v)
		}
	default:
		return fmt.Errorf("unknown front matter key %q", key)
	}
	return err
}

func scalar(v string) (string, error) {
	if strings.HasPrefix(v, `"`) {
		return strconv.Unquote(v)
	}
	return v, nil
}

func list(v string) ([]string, error) {
	if !strings.HasPrefix(v, "[") || !strings.HasSuffix(v, "]") {
		return nil, fmt.Errorf("list %q must be written as [a, b]", v)
	}
	var out []string
	var cur strings.Builder
	quoted, esc := false, false
	push := func() error {
		s, err := scalar(strings.TrimSpace(cur.String()))
		cur.Reset()
		if err != nil {
			return err
		}
		if s != "" {
			out = append(out, s)
		}
		return nil
	}
	for _, c := range v[1 : len(v)-1] {
		switch {
		case esc:
			esc = false
		case quoted && c == '\\':
			esc = true
		case c == '"':
			quoted = !quoted
		case c == ',' && !quoted:
			if err := push(); err != nil {
				return nil, err
			}
			continue
		}
		cur.WriteRune(c)
	}
	if quoted {
		return nil, fmt.Errorf("list %q has an open quote", v)
	}
	if err := push(); err != nil {
		return nil, err
	}
	return out, nil
}

func heading(md string) string {
	for _, l := range strings.Split(md, "\n") {
		if t, ok := strings.CutPrefix(l, "# "); ok {
			return strings.TrimSpace(t)
		}
	}
	return ""
}

func unwrap(l string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(l, '[')
		if i < 0 {
			break
		}
		j := strings.Index(l[i:], "](")
		if j < 0 {
			break
		}
		j += i
		if strings.IndexByte(l[i+1:j], '[') >= 0 {
			b.WriteString(l[:i+1])
			l = l[i+1:]
			continue
		}
		k := strings.IndexByte(l[j+2:], ')')
		if k < 0 {
			break
		}
		k += j + 2
		b.WriteString(l[:i])
		b.WriteString(l[i+1 : j])
		l = l[k+1:]
	}
	b.WriteString(l)
	return b.String()
}

func summary(md string) string {
	fence := false
	for _, l := range strings.Split(md, "\n") {
		if isFence(l) {
			fence = !fence
			continue
		}
		t := strings.TrimSpace(l)
		if fence || t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "|") || strings.HasPrefix(t, "![") && strings.HasSuffix(t, ")") {
			continue
		}
		return unwrap(trimMarker(t))
	}
	return ""
}

func trimMarker(t string) string {
	for _, p := range []string{"- ", "* ", "+ ", "> "} {
		if strings.HasPrefix(t, p) {
			return strings.TrimSpace(t[len(p):])
		}
	}
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	if i > 0 && strings.HasPrefix(t[i:], ". ") {
		return strings.TrimSpace(t[i+2:])
	}
	return t
}

func refs(md string) []string {
	var out []string
	fence := false
	for _, l := range strings.Split(md, "\n") {
		if isFence(l) {
			fence = !fence
			continue
		}
		if fence {
			continue
		}
		for {
			i := strings.Index(l, "](ref:")
			if i < 0 {
				break
			}
			rest := l[i+len("](ref:"):]
			j := strings.IndexByte(rest, ')')
			if j < 0 {
				break
			}
			out = append(out, rest[:j])
			l = rest[j+1:]
		}
	}
	return out
}

func snippets(md string) []Snippet {
	var out []Snippet
	var cur *Snippet
	var code []string
	for i, l := range strings.Split(md, "\n") {
		if isFence(l) {
			if cur == nil {
				info := strings.Fields(strings.TrimPrefix(strings.TrimSpace(l), "```"))
				s := Snippet{Line: i + 1}
				if len(info) > 0 {
					s.Lang = info[0]
					s.Run = slices.Contains(info[1:], "run")
					s.Check = slices.Contains(info[1:], "check")
				}
				cur, code = &s, code[:0]
				continue
			}
			cur.Code = strings.Join(code, "\n")
			out = append(out, *cur)
			cur = nil
			continue
		}
		if cur != nil {
			code = append(code, l)
		}
	}
	return out
}

func Plain(md string) string {
	var b strings.Builder
	fence := false
	first := true
	for _, l := range strings.Split(md, "\n") {
		if isFence(l) {
			fence = !fence
			continue
		}
		if !first {
			b.WriteByte('\n')
		}
		first = false
		if fence {
			b.WriteString(l)
			continue
		}
		b.WriteString(unwrap(l))
	}
	return b.String()
}

// HideChecks drops the aescene check blocks: docs check plays them after each step, readers do not need them.
func HideChecks(md string) string {
	if !strings.Contains(md, "aescene check") {
		return md
	}
	lines := strings.Split(md, "\n")
	out := lines[:0:0]
	skip, fence := false, false
	for _, l := range lines {
		if isFence(l) {
			if !fence {
				info := strings.Fields(strings.TrimPrefix(strings.TrimSpace(l), "```"))
				skip = len(info) > 1 && info[0] == "aescene" && slices.Contains(info[1:], "check")
			}
			fence = !fence
			if skip {
				if !fence {
					skip = false
				}
				continue
			}
		} else if skip {
			continue
		}
		out = append(out, l)
	}
	return collapseBlank(strings.Join(out, "\n"))
}

func collapseBlank(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimRight(s, "\n")
}
