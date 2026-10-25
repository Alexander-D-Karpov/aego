package site

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var mdRef = regexp.MustCompile(`\]\(ref:([^)#\s]+)(#[^)\s]*)?\)`)

// mdURL is the Markdown copy of a page next to its HTML: page.html becomes page.md.
func mdURL(rel string) string {
	return strings.TrimSuffix(rel, ".html") + ".md"
}

// pageMD is a page as Markdown for agents: its text with links to other pages' Markdown, then the fields, methods and other members the HTML page lists.
func (b *builder) pageMD(e *Entry) string {
	var c strings.Builder
	body := strings.TrimSpace(e.Body)
	if !strings.HasPrefix(body, "# ") {
		c.WriteString("# " + e.Title + "\n\n")
	}
	if e.Signature != "" && e.Kind != "page" {
		c.WriteString("```\n" + e.Signature + "\n```\n\n")
	}
	if body != "" {
		c.WriteString(b.mdLinks(body) + "\n")
	}
	var fields, members []*Entry
	for _, k := range b.kids[e.ID] {
		ke := b.byID[k]
		if u := b.urls[k]; u != "" && !strings.Contains(u, "#") {
			continue
		}
		if ke.Field != nil {
			fields = append(fields, ke)
		} else {
			members = append(members, ke)
		}
	}
	slices.SortStableFunc(fields, func(a, b *Entry) int { return a.Field.Index - b.Field.Index })
	if len(fields) > 0 {
		c.WriteString("\n## Fields\n\n| Name | Script name | Type | Default | Description |\n|---|---|---|---|---|\n")
		for _, f := range fields {
			label := f.Field.Label
			if label == "" {
				label = f.Title
			}
			desc := strings.ReplaceAll(strings.TrimSpace(b.mdLinks(f.Body)), "\n", " ")
			if f.Field.Runtime {
				desc = strings.TrimSpace("Runtime state. " + desc)
			}
			c.WriteString("| " + label + " | `" + f.Title + "` | `" + f.Field.Type + "` | " + backticked(f.Field.Default) + " | " + strings.ReplaceAll(desc, "|", `\|`) + " |\n")
		}
	}
	for _, m := range members {
		c.WriteString("\n## " + m.Title + "\n\n")
		if m.Signature != "" {
			c.WriteString("```\n" + m.Signature + "\n```\n\n")
		}
		if t := strings.TrimSpace(m.Body); t != "" {
			c.WriteString(b.mdLinks(t) + "\n")
		}
	}
	return c.String()
}

func backticked(s string) string {
	if s == "" {
		return ""
	}
	return "`" + s + "`"
}

// mdLinks points ref: links at the Markdown copies, from the site root.
func (b *builder) mdLinks(md string) string {
	return mdRef.ReplaceAllStringFunc(md, func(m string) string {
		sub := mdRef.FindStringSubmatch(m)
		u, ok := b.urls[sub[1]]
		if !ok {
			if e, ok := b.byID[sub[1]]; ok {
				if pu, ok := b.urls[e.Parent]; ok {
					return "](/" + mdURL(pu) + "#" + e.ID + ")"
				}
			}
			return m
		}
		return "](/" + mdURL(u) + sub[2] + ")"
	})
}

// writeMarkdown writes page.md next to every page and llms-full.txt with all of them in navigation order.
func (b *builder) writeMarkdown() error {
	var full strings.Builder
	full.WriteString("# " + b.exp.Title + "\n\n> " + homeLead + "\n")
	for _, id := range b.order {
		e := b.byID[id]
		rel := mdURL(b.urls[id])
		md := b.pageMD(e)
		p := filepath.Join(b.out, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(md), 0o644); err != nil {
			return err
		}
		full.WriteString("\n---\n\nSource: /" + rel + "\n\n" + md)
	}
	return os.WriteFile(filepath.Join(b.out, "llms-full.txt"), []byte(full.String()), 0o644)
}
