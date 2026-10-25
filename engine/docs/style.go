//go:build !aego_runtime

package docs

import (
	_ "embed"
	"go/format"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"aego/format/aetext"
)

// Limits of the style checks (D347).
const (
	maxParagraphWords = 120
	maxDescChars      = 160
	maxDescSentences  = 2
	dupSentenceWords  = 12
)

// lintSource is the list of patterns DOC010 reports; it is embedded so an installed aego checks without the repository.
//
//go:embed lint.aetext
var lintSource []byte

var (
	decisionRe = regexp.MustCompile(`\bD\d{3}\b`)
	packageRe  = regexp.MustCompile(`\b(?:engine|editor|scene|core|modules/[a-z0-9_]+)/[A-Za-z0-9_./-]*`)
	pseudoHead = regexp.MustCompile(`^\*\*[^*\n]+(?:[.:]\*\*|\*\*[.:])\s+\S`)
	inlineCode = regexp.MustCompile("`[^`]*`")
	linkTarget = regexp.MustCompile(`\]\([^)]*\)`)
	nonWord    = regexp.MustCompile(`[^\p{L}\p{N}]+`)
	symbolLike = regexp.MustCompile(`^(?:[a-z][a-z0-9]*\.)?[A-Z][A-Za-z0-9]*(?:\.[A-Z][A-Za-z0-9]*)*(?:\(\))?$`)
	codeSpan   = regexp.MustCompile("`([^`]+)`")
	fixMarker  = regexp.MustCompile(`(?m)^(?:\*\*Fix\.\*\*|Fix:)\s*`)
)

// TrimSelf drops a leading name from a Go-style doc comment: "ApplyForce pushes" becomes "Pushes"; "Mass in kilograms" stays, the name is not followed by a verb.
func TrimSelf(name, doc string) string {
	doc = strings.TrimSpace(doc)
	rest, ok := strings.CutPrefix(doc, name)
	if !ok || rest == "" || rest[0] != ' ' {
		return doc
	}
	rest = strings.TrimLeft(rest, " ")
	r, n := utf8.DecodeRuneInString(rest)
	if r == utf8.RuneError || !verb(rest) {
		return doc
	}
	return string(unicode.ToUpper(r)) + rest[n:]
}

// verb reports whether text starts with a lowercase word in the third person: pushes, runs, is, has.
func verb(text string) bool {
	w, _, _ := strings.Cut(text, " ")
	w = strings.TrimRight(w, ",;:.")
	if len(w) < 2 || !unicode.IsLower(rune(w[0])) {
		return false
	}
	return strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && !strings.HasSuffix(w, "us")
}

// spaced is the display form of a Go name: "MassMode" becomes "Mass Mode".
func spaced(name string) string {
	var b strings.Builder
	prev := rune(0)
	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev)) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
		prev = r
	}
	return b.String()
}

// trimSelfDoc strips the entry's own name from the start of its body, in Go and display form.
func trimSelfDoc(e *Entry) {
	switch e.Kind {
	case Field, Method, Signal, Function:
	default:
		return
	}
	names := []string{strings.TrimSuffix(e.Title, "()")}
	if i := strings.LastIndexByte(e.ID, '.'); i >= 0 {
		names = append(names, strings.TrimSuffix(e.ID[i+1:], "()"))
	}
	for _, n := range names {
		if s := spaced(n); s != n {
			names = append(names, s)
		}
	}
	for _, n := range names {
		if t := TrimSelf(n, e.Body); t != strings.TrimSpace(e.Body) {
			e.Body = t
			return
		}
	}
}

// firstSentence is the text up to the first sentence end followed by a capital letter or the end.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '.' && c != '?' && c != '!' {
			continue
		}
		if i+1 == len(s) {
			return s
		}
		if s[i+1] != ' ' && s[i+1] != '\n' || c == '.' && i >= 2 && s[i-2] == '.' {
			continue
		}
		r, _ := utf8.DecodeRuneInString(strings.TrimLeft(s[i+1:], " \n"))
		if unicode.IsUpper(r) || unicode.IsDigit(r) || r == '`' || r == '*' {
			return s[:i+1]
		}
	}
	return s
}

func sentences(s string) int {
	n := 0
	for rest := strings.TrimSpace(s); rest != ""; n++ {
		f := firstSentence(rest)
		rest = strings.TrimSpace(rest[len(f):])
	}
	return n
}

type paragraph struct {
	raw  string
	text string
	line int
}

// prose splits a page into paragraphs and list items outside code blocks, tables and headings; text has inline code and link targets removed.
func prose(md string) []paragraph {
	var out []paragraph
	var cur []string
	start := 0
	flush := func() {
		if len(cur) == 0 {
			return
		}
		raw := strings.Join(cur, " ")
		text := linkTarget.ReplaceAllString(inlineCode.ReplaceAllString(raw, ""), "]")
		out = append(out, paragraph{raw: raw, text: text, line: start})
		cur = nil
	}
	fence := false
	for i, l := range strings.Split(md, "\n") {
		t := strings.TrimSpace(l)
		if isFence(l) {
			flush()
			fence = !fence
			continue
		}
		if fence {
			continue
		}
		switch {
		case t == "", strings.HasPrefix(t, "#"), strings.HasPrefix(t, "|"), strings.HasPrefix(t, "<"), strings.HasPrefix(t, "!["):
			flush()
			continue
		case strings.HasPrefix(t, "- "), strings.HasPrefix(t, "* "), listNumber(t):
			flush()
			start = i
			cur = append(cur, trimMarker(t))
			continue
		}
		if len(cur) == 0 {
			start = i
		}
		cur = append(cur, t)
	}
	flush()
	return out
}

func listNumber(t string) bool {
	n := 0
	for n < len(t) && t[n] >= '0' && t[n] <= '9' {
		n++
	}
	return n > 0 && n+1 < len(t) && t[n] == '.' && t[n+1] == ' '
}

type lintRule struct {
	re   *regexp.Regexp
	src  string
	skip string
	why  string
}

// lintRules parses lint.aetext; the file ships with the binary, so a broken one is a build bug caught by the tests.
func lintRules() ([]lintRule, error) {
	doc, err := aetext.Parse(lintSource, "lint.aetext")
	if err != nil {
		return nil, err
	}
	var out []lintRule
	for _, b := range doc.Blocks {
		if b.Head != "pattern" {
			continue
		}
		src, _ := b.Field("regex")
		re, err := regexp.Compile("(?i)" + src.Str)
		if err != nil {
			return nil, err
		}
		skip, _ := b.Field("skip_group")
		why, _ := b.Field("why")
		out = append(out, lintRule{re: re, src: src.Str, skip: skip.Str, why: why.Str})
	}
	return out, nil
}

// guide is a guide or concept page; the language reference keeps its own shape (D347).
func (r *Registry) guide(e *Entry) bool {
	if e.Kind != Page || e.EditorOnly || e.Group == GroupScript {
		return false
	}
	g, ok := r.group(e.Group)
	return ok && g.Section != SectionReference
}

func (r *Registry) prosePage(e *Entry) bool {
	switch e.Kind {
	case Page, Module, Code:
		return true
	case Type:
		return e.Component != nil
	}
	return false
}

// intro is the paragraph between the title and the first section heading, or "" when the page starts with a heading, a list or code.
func intro(md string) string {
	fence := false
	for _, l := range strings.Split(md, "\n") {
		t := strings.TrimSpace(l)
		switch {
		case isFence(l):
			return ""
		case fence, t == "", strings.HasPrefix(t, "# "), strings.HasPrefix(t, "!["):
			continue
		case strings.HasPrefix(t, "#"), strings.HasPrefix(t, "|"), strings.HasPrefix(t, "- "), strings.HasPrefix(t, "* "), listNumber(t), strings.HasPrefix(t, "<"):
			return ""
		}
		for _, p := range prose(md) {
			if strings.Contains(p.raw, t) {
				return p.text
			}
		}
		return t
	}
	return ""
}

// IntroWords is the length of a page's opening paragraph in words.
func IntroWords(md string) int {
	return len(strings.Fields(intro(md)))
}

// GoSnippets are the go code blocks of a page.
func GoSnippets(e *Entry) []Snippet {
	var out []Snippet
	for _, s := range e.Snippets {
		if s.Lang == "go" {
			out = append(out, s)
		}
	}
	return out
}

// gofmtGo reports a go block that gofmt would change; a fragment is wrapped in a function first.
func gofmtGo(code string) string {
	src := []byte(code)
	if !strings.HasPrefix(strings.TrimSpace(code), "package ") {
		src = []byte("package p\n\nfunc _() {\n" + code + "\n}\n")
		if _, err := format.Source(src); err != nil {
			src = []byte("package p\n\n" + code + "\n")
		}
	}
	if _, err := format.Source(src); err != nil {
		return "does not parse: " + err.Error()
	}
	return ""
}

func (r *Registry) inGroup(e *Entry, group string) bool {
	return group != "" && e.Group == group
}

// StyleProblems are the D347 style rules; they are errors only in strict mode, except a component field without a description, which always is.
func (r *Registry) StyleProblems(strict bool) []Problem {
	var out []Problem
	add := func(e *Entry, line int, code, msg string) {
		at := ""
		if line > 0 {
			at = " (line " + strconv.Itoa(line) + ")"
		}
		out = append(out, Problem{ID: e.ID, Error: strict, Message: code + " " + msg + at})
	}
	rules, err := lintRules()
	if err != nil {
		return []Problem{{ID: "lint.aetext", Error: true, Message: err.Error()}}
	}
	seen := map[string]string{}
	aliases := r.Aliases()
	names, roots := map[string]bool{}, map[string]bool{}
	for i := range r.entries {
		id := strings.TrimSuffix(r.entries[i].ID, "()")
		names[id[strings.LastIndexAny(id, "./")+1:]] = true
		if j := strings.IndexByte(id, '.'); j > 0 {
			roots[id[:j]] = true
		}
	}
	for i := range r.entries {
		e := &r.entries[i]
		switch {
		case r.prosePage(e):
			allow := r.meta[e.ID].allow
			for _, p := range prose(e.Body) {
				line := 0
				if e.Location.Line > 0 {
					line = e.Location.Line + p.line + 1
				}
				for _, rule := range rules {
					if r.inGroup(e, rule.skip) {
						continue
					}
					if m := rule.re.FindString(p.text); m != "" && !allowed(allow, m) {
						add(e, line, "DOC010", "matches "+strconv.Quote(rule.src)+": "+strconv.Quote(m)+", "+rule.why)
					}
				}
				if m := decisionRe.FindString(p.text); m != "" {
					add(e, line, "DOC011", "names decision "+m+"; implementation notes belong in ARCHITECTURE.md")
				} else if m := packageRe.FindString(p.text); m != "" && !strings.Contains(m, "/res/") {
					add(e, line, "DOC011", "names internal path "+m)
				}
				if n := len(strings.Fields(p.text)); n > maxParagraphWords {
					add(e, line, "DOC012", "paragraph of "+strconv.Itoa(n)+" words, at most "+strconv.Itoa(maxParagraphWords))
				}
				if pseudoHead.MatchString(p.raw) && !(e.Kind == Code && fixMarker.MatchString(p.raw)) {
					add(e, line, "DOC013", "paragraph starts with a bold phrase; make it a heading")
				}
				for _, m := range codeSpan.FindAllStringSubmatch(p.raw, -1) {
					id := m[1]
					bare := strings.TrimSuffix(id, "()")
					if !symbolLike.MatchString(id) || !strings.Contains(id, ".") && bare == id {
						continue
					}
					if first, _, ok := strings.Cut(bare, "."); ok && (first[0] >= 'a' && first[0] <= 'z' && !roots[first] || first[0] >= 'A' && first[0] <= 'Z' && !names[first]) {
						continue
					}
					if _, ok := r.Link(id); ok {
						continue
					}
					if _, ok := aliases[bare]; ok || names[bare[strings.LastIndexByte(bare, '.')+1:]] {
						continue
					}
					add(e, line, "DOC020", "`"+id+"` looks like a component, function or field but names nothing in the reference")
				}
				if e.Kind != Page {
					continue
				}
				for rest := strings.TrimSpace(p.text); rest != ""; {
					s := firstSentence(rest)
					rest = strings.TrimSpace(rest[len(s):])
					words := strings.Fields(strings.ToLower(nonWord.ReplaceAllString(s, " ")))
					if len(words) < dupSentenceWords {
						continue
					}
					key := strings.Join(words, " ")
					if other, ok := seen[key]; ok && other != e.ID {
						add(e, line, "DOC017", "repeats a sentence of "+other+": "+strconv.Quote(clip(s, 80)))
					} else if !ok {
						seen[key] = e.ID
					}
				}
			}
			if r.guide(e) && intro(e.Body) == "" {
				add(e, e.Location.Line, "DOC019", "has no opening paragraph before its first section; say what it is and when it is needed")
			}
			for _, sn := range GoSnippets(e) {
				if msg := gofmtGo(sn.Code); msg != "" {
					add(e, e.Location.Line+sn.Line, "DOC022", "go block "+msg)
				}
			}
			if e.Kind == Code {
				r.diagShape(e, add)
			}
		case e.Kind == Field && e.Field != nil:
			body := strings.TrimSpace(e.Body)
			if body == "" {
				out = append(out, Problem{ID: e.ID, Error: true, Message: "DOC021 component field has no description; give it a FieldDoc"})
				continue
			}
			if len(body) > maxDescChars || sentences(body) > maxDescSentences {
				add(e, 0, "DOC014", "field description of "+strconv.Itoa(len(body))+" characters and "+strconv.Itoa(sentences(body))+" sentences, at most "+strconv.Itoa(maxDescChars)+" and "+strconv.Itoa(maxDescSentences))
			}
			if e.Field.Runtime && strings.Contains(strings.ToLower(body), "during play") {
				add(e, 0, "DOC015", "runtime field says \"during Play\"; it is listed under Runtime state")
			}
		case e.Kind == Method || e.Kind == Signal || e.Kind == Function:
			if s := firstSentence(e.Body); len(s) > maxDescChars {
				add(e, 0, "DOC016", "first sentence of "+strconv.Itoa(len(s))+" characters, at most "+strconv.Itoa(maxDescChars))
			}
		}
	}
	return out
}

// diagShape holds a diagnostic page to one sentence on what happened and one on the fix, before its example.
func (r *Registry) diagShape(e *Entry, add func(*Entry, int, string, string)) {
	body := e.Body
	for _, cut := range []string{"\n## ", "\n```"} {
		if i := strings.Index(body, cut); i >= 0 {
			body = body[:i]
		}
	}
	loc := fixMarker.FindStringIndex(body)
	if loc == nil {
		add(e, e.Location.Line, "DOC023", "diagnostic has no **Fix.** sentence")
		return
	}
	what := strings.Join(strings.Fields(plainProse(body[:loc[0]])), " ")
	fix := strings.Join(strings.Fields(plainProse(body[loc[1]:])), " ")
	if n := sentences(what); n != 1 {
		add(e, e.Location.Line, "DOC023", "diagnostic says what happened in "+strconv.Itoa(n)+" sentences, want one")
	}
	if n := sentences(fix); n != 1 {
		add(e, e.Location.Line, "DOC023", "diagnostic says how to fix it in "+strconv.Itoa(n)+" sentences, want one")
	}
}

func plainProse(md string) string {
	var b strings.Builder
	for _, p := range prose(md) {
		b.WriteString(p.text + " ")
	}
	return b.String()
}

func allowed(list []string, m string) bool {
	for _, a := range list {
		if strings.EqualFold(a, m) {
			return true
		}
	}
	return false
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
