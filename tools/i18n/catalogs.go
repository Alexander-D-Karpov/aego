package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"aego/engine/i18n"
)

const engineDir = "engine/i18n/messages"

// catalogDir is where a key lives: module keys i18n.<m>.* in modules/<m>/res/i18n, the rest in the engine.
func catalogDir(key string) string {
	for _, p := range []string{"schema.", "enum.", "setting.", "diag.", "api.", "docs.", "cli.agent."} {
		if strings.HasPrefix(key, p) {
			return dataDir
		}
	}
	if rest, ok := strings.CutPrefix(key, "i18n."); ok {
		m, _, _ := strings.Cut(rest, ".")
		return filepath.Join("modules", m, "res", "i18n")
	}
	return engineDir
}

func group(keys map[string]*Key) map[string][]*Key {
	out := map[string][]*Key{engineDir: nil, dataDir: nil}
	for _, k := range keys {
		d := catalogDir(k.ID)
		out[d] = append(out[d], k)
	}
	for _, ks := range out {
		sort.Slice(ks, func(i, j int) bool { return ks[i].ID < ks[j].ID })
	}
	return out
}

// Extract rewrites each en catalog from the code.
func Extract(root string, keys map[string]*Key) (int, error) {
	groups := group(keys)
	for dir, ks := range groups {
		c := &i18n.Catalog{Lang: i18n.English, Msgs: map[string]i18n.Message{}}
		for _, k := range ks {
			m := i18n.Message{Text: k.En}
			if k.Plural {
				m = i18n.Message{Plural: map[i18n.Form]string{i18n.One: k.One, i18n.Other: k.En}}
			}
			c.Msgs[k.ID] = m
			c.Order = append(c.Order, k.ID)
		}
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return 0, err
		}
		if err := os.WriteFile(filepath.Join(root, dir, "en.aetext"), i18n.Write(c), 0o644); err != nil {
			return 0, err
		}
	}
	return len(groups), nil
}

// Finding is one problem of a translated catalog.
type Finding struct {
	Code string
	Lang string
	Key  string
	Msg  string
}

// Report is the result of check.
type Report struct {
	Keys     int
	Langs    map[string]int
	Missing  map[string]int
	Findings []Finding
	Stale    []string
}

// Check compares every non-English catalog with the keys of the code.
func Check(root string, keys map[string]*Key) (*Report, error) {
	r := &Report{Keys: len(keys), Langs: map[string]int{}, Missing: map[string]int{}}
	for dir, ks := range group(keys) {
		en, err := read(filepath.Join(root, dir, "en.aetext"))
		if err != nil {
			return nil, err
		}
		if en == nil || !sameKeys(en, ks) {
			r.Stale = append(r.Stale, filepath.ToSlash(filepath.Join(dir, "en.aetext")))
		}
		files, _ := filepath.Glob(filepath.Join(root, dir, "*.aetext"))
		sort.Strings(files)
		for _, f := range files {
			if filepath.Base(f) == "en.aetext" {
				continue
			}
			c, err := read(f)
			if err != nil {
				return nil, err
			}
			r.Langs[c.Lang] += len(c.Msgs)
			byID := map[string]*Key{}
			for _, k := range ks {
				byID[k.ID] = k
				m, ok := c.Msgs[k.ID]
				if !ok {
					r.Missing[c.Lang]++
					r.Findings = append(r.Findings, Finding{i18n.MissingKey, c.Lang, k.ID, "no translation (" + k.Pos + ")"})
					continue
				}
				r.Findings = append(r.Findings, compare(c.Lang, k, m)...)
			}
			for _, id := range c.Order {
				if byID[id] == nil {
					r.Findings = append(r.Findings, Finding{i18n.UnusedKey, c.Lang, id, "the code has no such key"})
				}
			}
		}
	}
	langs := map[string]bool{}
	for dir := range group(keys) {
		files, _ := filepath.Glob(filepath.Join(root, dir, "*.aetext"))
		for _, f := range files {
			if b := filepath.Base(f); b != "en.aetext" {
				langs[strings.TrimSuffix(b, ".aetext")] = true
			}
		}
	}
	for dir, ks := range group(keys) {
		for lang := range langs {
			if _, err := os.Stat(filepath.Join(root, dir, lang+".aetext")); err == nil || len(ks) == 0 {
				continue
			}
			for _, k := range ks {
				r.Missing[lang]++
				r.Findings = append(r.Findings, Finding{i18n.MissingKey, lang, k.ID, "no " + lang + ".aetext in " + filepath.ToSlash(dir)})
			}
		}
	}
	sort.Slice(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Key < b.Key
	})
	return r, nil
}

func compare(lang string, k *Key, m i18n.Message) []Finding {
	var out []Finding
	want := set(i18n.Placeholders(k.En + " " + k.One))
	check := func(text string) {
		got := set(i18n.Placeholders(text))
		for p := range got {
			if !want[p] && !(k.Plural && p == "n") {
				out = append(out, Finding{i18n.BadPlaceholder, lang, k.ID, fmt.Sprintf("{%s} is not in the English text", p)})
			}
		}
		for p := range want {
			if !got[p] && !(k.Plural && p == "n") {
				out = append(out, Finding{i18n.BadPlaceholder, lang, k.ID, fmt.Sprintf("{%s} is missing", p)})
			}
		}
	}
	if k.Plural {
		if m.Plural == nil {
			return append(out, Finding{i18n.BadPlaceholder, lang, k.ID, "needs plural forms, has text"})
		}
		for _, f := range forms(lang) {
			t, ok := m.Plural[f]
			if !ok {
				out = append(out, Finding{i18n.BadPlaceholder, lang, k.ID, "no " + f.String() + " form"})
				continue
			}
			check(t)
		}
		return out
	}
	if m.Plural != nil || m.Text == "" {
		return append(out, Finding{i18n.BadPlaceholder, lang, k.ID, "needs a text"})
	}
	check(m.Text)
	return out
}

func forms(lang string) []i18n.Form {
	if lang == i18n.Russian {
		return []i18n.Form{i18n.One, i18n.Few, i18n.Many}
	}
	return []i18n.Form{i18n.One, i18n.Other}
}

func set(xs []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func read(path string) (*i18n.Catalog, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return i18n.Parse(data, path)
}

func sameKeys(c *i18n.Catalog, ks []*Key) bool {
	if len(c.Msgs) != len(ks) {
		return false
	}
	for _, k := range ks {
		m, ok := c.Msgs[k.ID]
		if !ok || (!k.Plural && m.Text != k.En) || (k.Plural && (m.Plural[i18n.One] != k.One || m.Plural[i18n.Other] != k.En)) {
			return false
		}
	}
	return true
}

// Failed is true for stale en catalogs, unused keys and bad placeholders, and with strict for missing ones too.
func (r *Report) Failed(strict bool) bool {
	if len(r.Stale) > 0 {
		return true
	}
	for _, f := range r.Findings {
		if f.Code != i18n.MissingKey || strict {
			return true
		}
	}
	return false
}

func (r *Report) Print(w io.Writer) {
	for _, s := range r.Stale {
		fmt.Fprintf(w, "STALE %s: run go run ./tools/i18n extract\n", s)
	}
	for _, f := range r.Findings {
		fmt.Fprintf(w, "%s %s %s: %s\n", f.Code, f.Lang, f.Key, f.Msg)
	}
	langs := make([]string, 0, len(r.Langs))
	for l := range r.Langs {
		langs = append(langs, l)
	}
	sort.Strings(langs)
	fmt.Fprintf(w, "i18n check: en %d keys", r.Keys)
	for _, l := range langs {
		fmt.Fprintf(w, ", %s %d keys, %d without a translation", l, r.Langs[l], r.Missing[l])
	}
	fmt.Fprintln(w)
}
