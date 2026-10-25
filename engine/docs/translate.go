//go:build !aego_runtime

package docs

import (
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"aego/engine/module"
)

// Translation is an entry in another language: a page from its name.<lang>.md file, or a reference entry from the catalogs (D348).
type Translation struct {
	Title   string `json:",omitempty"`
	Summary string `json:",omitempty"`
	Body    string `json:",omitempty"`
	// Fix is a diagnostic's fix sentence.
	Fix string `json:",omitempty"`
	// Source is the translated file, for the site's edit link; empty when the text comes from a catalog.
	Source string `json:",omitempty"`
	Line   int    `json:",omitempty"`
}

var translationFile = regexp.MustCompile(`\.([a-z]{2})\.md$`)

// translationLang is the language of a file named name.<lang>.md, or "".
func translationLang(file string) string {
	if m := translationFile.FindStringSubmatch(file); m != nil {
		return m[1]
	}
	return ""
}

// translations reads the name.<lang>.md files of a source into the entries they translate.
func (r *Registry) translations(src module.DocSource, files []string) {
	for _, f := range files {
		lang := translationLang(f)
		data, err := fs.ReadFile(src.FS, f)
		if err != nil {
			r.problems = append(r.problems, Problem{ID: src.Owner + ":" + f, Error: true, Message: err.Error()})
			continue
		}
		recs, err := parseRecords(string(data))
		if err != nil {
			r.problems = append(r.problems, Problem{ID: src.Owner + ":" + f, Error: true, Message: err.Error()})
			continue
		}
		source := ""
		if src.Dir != "" {
			source = path.Join(src.Dir, f)
		}
		for _, rec := range recs {
			id := rec.id
			if rec.code != "" {
				id = rec.code
			}
			i, ok := r.byID[id]
			if !ok {
				r.problems = append(r.problems, Problem{ID: src.Owner + ":" + f, Error: true, Message: "translation of " + id + ", which no page declares"})
				continue
			}
			e := &r.entries[i]
			if e.Translations == nil {
				e.Translations = map[string]Translation{}
			}
			t := e.Translations[lang]
			t.Body = joinBody(t.Body, rec.body)
			if rec.title != "" {
				t.Title = rec.title
			} else if t.Title == "" {
				t.Title = heading(rec.body)
			}
			if rec.summary != "" {
				t.Summary = rec.summary
			}
			if t.Source == "" {
				t.Source, t.Line = source, rec.line
			}
			e.Translations[lang] = t
			if rec.groupTitle != "" {
				r.groupTitles(rec.group, lang, rec.groupTitle)
			}
		}
	}
}

func (r *Registry) groupTitles(id, lang, title string) {
	if r.groupT == nil {
		r.groupT = map[string]map[string]string{}
	}
	if r.groupT[id] == nil {
		r.groupT[id] = map[string]string{}
	}
	r.groupT[id][lang] = title
}

// Localize fills the reference entries of lang from the catalogs through tr: components, fields, script functions, settings, script diagnostics and the builtin groups. Pages keep their file translations.
func (r *Registry) Localize(lang string, tr func(key, en string) string) {
	set := func(e *Entry, t Translation) {
		if t == (Translation{}) {
			return
		}
		if e.Translations == nil {
			e.Translations = map[string]Translation{}
		}
		if f, file := e.Translations[lang]; file {
			if f.Title == "" {
				f.Title = t.Title
			}
			if e.Kind == Type {
				f.Body = joinBody(t.Body, f.Body)
			}
			e.Translations[lang] = f
			return
		}
		e.Translations[lang] = t
	}
	keep := func(key, en string) string {
		if en == "" {
			return ""
		}
		if t := tr(key, en); t != en {
			return t
		}
		return ""
	}
	for i := range r.entries {
		e := &r.entries[i]
		switch {
		case e.Kind == Type && e.Component != nil:
			set(e, Translation{Title: keep("schema."+e.ID, e.Title), Body: keep("schema."+e.ID+".doc", e.Body)})
		case strings.HasPrefix(e.ID, SettingPrefix):
			id := strings.TrimPrefix(e.ID, SettingPrefix)
			set(e, Translation{Title: keep("setting."+id, e.Title), Body: keep("setting."+id+".doc", e.Body)})
		case e.Kind == Field && e.Field != nil:
			key := "schema." + e.Parent + "." + lowerFirst(e.Title)
			set(e, Translation{Title: keep(key, e.Field.Label), Body: keep(key+".doc", e.Body)})
		case e.Kind == Code && e.Diag != nil && e.Diag.Fix != "":
			what, fix := keep("diag."+e.ID, e.Summary), keep("diag."+e.ID+".fix", e.Diag.Fix)
			if what != "" {
				set(e, Translation{Summary: what, Body: what + "\n\n" + tr("docs.fix", "Fix:") + " " + fix, Fix: fix})
			}
		case e.Kind == Function || e.Kind == Method || e.Kind == Signal || e.Kind == Const || e.Kind == Enum:
			set(e, Translation{Body: keep("api."+e.ID, e.Body)})
		}
	}
	for _, g := range groups {
		if t := keep("docs.group."+g.ID, g.Title); t != "" {
			r.groupTitles(g.ID, lang, t)
		}
	}
}

func lowerFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToLower(r)) + s[n:]
}

var refTarget = regexp.MustCompile(`\]\(ref:([^)\s#]+)`)

// LangProblems checks the translations into lang: every page has one, and each keeps the code blocks, the headings and valid ref: links of its original (D348).
func (r *Registry) LangProblems(lang string, strict bool) []Problem {
	var out []Problem
	for i := range r.entries {
		e := &r.entries[i]
		if (e.Kind != Page && e.Kind != Code) || e.Location.Source == "" || e.EditorOnly {
			continue
		}
		if e.Kind == Code && e.Diag != nil && e.Diag.Fix != "" {
			continue
		}
		t, ok := e.Translations[lang]
		if !ok || t.Source == "" {
			out = append(out, Problem{ID: e.ID, Error: strict, Message: "no " + lang + " translation (" + translationName(e.Location.Source, lang) + ")"})
			continue
		}
		at := t.Source
		if a, b := codeBlocks(e.Body), codeBlocks(t.Body); !slices.Equal(a, b) {
			out = append(out, Problem{ID: e.ID, Error: true, Message: at + ": the code blocks differ from the English page (" + strconv.Itoa(len(b)) + " of " + strconv.Itoa(len(a)) + " match in order); copy them unchanged"})
		}
		if a, b := headingCount(e.Body), headingCount(t.Body); a != b {
			out = append(out, Problem{ID: e.ID, Error: true, Message: at + ": " + strconv.Itoa(b) + " headings, the English page has " + strconv.Itoa(a)})
		}
		for _, m := range refTarget.FindAllStringSubmatch(t.Body, -1) {
			if _, ok := r.Link(m[1]); !ok {
				if _, ok := r.byID[m[1]]; !ok {
					out = append(out, Problem{ID: e.ID, Error: true, Message: at + ": broken link ref:" + m[1]})
				}
			}
		}
	}
	return out
}

func translationName(source, lang string) string {
	return strings.TrimSuffix(source, ".md") + "." + lang + ".md"
}

func codeBlocks(md string) []string {
	var out []string
	for _, s := range snippets(md) {
		out = append(out, s.Lang+"\n"+s.Code)
	}
	return out
}

func headingCount(md string) int {
	n, fence := 0, false
	for _, l := range strings.Split(md, "\n") {
		if isFence(l) {
			fence = !fence
			continue
		}
		if !fence && strings.HasPrefix(l, "#") {
			n++
		}
	}
	return n
}
