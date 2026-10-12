package richtext

import (
	"strconv"
	"strings"
)

type Style uint8

const (
	Plain Style = iota
	Strong
	Emphasis
	Code
)

type Span struct {
	Text   string
	Style  Style
	Target string
}

type Kind uint8

const (
	Paragraph Kind = iota
	Heading
	CodeBlock
	ListItem
	Table
	Rule
	Quote
)

type Block struct {
	Kind    Kind
	Level   int
	Ordinal int
	Spans   []Span
	Lang    string
	Info    string
	Lines   []string
	Rows    [][][]Span
	Header  bool
}

func (b *Block) Runnable() bool {
	if b.Kind != CodeBlock {
		return false
	}
	f := strings.Fields(b.Info)
	for _, w := range f[min(1, len(f)):] {
		if w == "run" {
			return true
		}
	}
	return false
}

func isFence(t string) bool {
	return strings.HasPrefix(t, "```")
}

func heading(t string) (int, string, bool) {
	n := 0
	for n < len(t) && n < 6 && t[n] == '#' {
		n++
	}
	if n == 0 || n >= len(t) || t[n] != ' ' {
		return 0, "", false
	}
	return n, strings.TrimSpace(t[n+1:]), true
}

func isRule(t string) bool {
	if len(t) < 3 {
		return false
	}
	c := t[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	for i := 0; i < len(t); i++ {
		if t[i] != c && t[i] != ' ' {
			return false
		}
	}
	return true
}

func listItem(raw string) (level, ordinal int, text string, ok bool) {
	indent := 0
	for indent < len(raw) && (raw[indent] == ' ' || raw[indent] == '\t') {
		if raw[indent] == '\t' {
			indent += 4
		} else {
			indent++
		}
	}
	t := strings.TrimLeft(raw, " \t")
	level = indent / 2
	if len(t) >= 2 && (t[0] == '-' || t[0] == '*' || t[0] == '+') && t[1] == ' ' {
		return level, 0, strings.TrimSpace(t[2:]), true
	}
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	if i > 0 && i+1 < len(t) && (t[i] == '.' || t[i] == ')') && t[i+1] == ' ' {
		n, _ := strconv.Atoi(t[:i])
		return level, n, strings.TrimSpace(t[i+2:]), true
	}
	return 0, 0, "", false
}

func indented(raw string) (string, bool) {
	switch {
	case strings.HasPrefix(raw, "\t"):
		return raw[1:], true
	case strings.HasPrefix(raw, "    "):
		return raw[4:], true
	}
	return "", false
}

func cells(t string) []string {
	t = strings.TrimSpace(t)
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	var out []string
	var cur strings.Builder
	code := false
	for i := 0; i < len(t); i++ {
		c := t[i]
		switch {
		case c == '\\' && i+1 < len(t) && t[i+1] == '|':
			cur.WriteByte('|')
			i++
			continue
		case c == '`':
			code = !code
		case c == '|' && !code:
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	return append(out, strings.TrimSpace(cur.String()))
}

func separatorRow(t string) bool {
	return strings.Trim(t, "|-: \t") == "" && strings.Contains(t, "-")
}

func Parse(md string) []Block {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var out []Block
	var pend *Block
	var text []string
	flush := func() {
		if pend != nil {
			pend.Spans = Inline(strings.Join(text, " "))
			out = append(out, *pend)
			pend, text = nil, text[:0]
		}
	}
	start := func(b Block, s string) {
		flush()
		pend = &b
		text = append(text, s)
	}
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		t := strings.TrimSpace(raw)
		if _, ok := indented(raw); ok && pend == nil && t != "" {
			b := Block{Kind: CodeBlock}
			for ; i < len(lines); i++ {
				s, ok := indented(lines[i])
				if !ok && strings.TrimSpace(lines[i]) != "" {
					break
				}
				b.Lines = append(b.Lines, s)
			}
			i--
			for len(b.Lines) > 0 && strings.TrimSpace(b.Lines[len(b.Lines)-1]) == "" {
				b.Lines = b.Lines[:len(b.Lines)-1]
			}
			out = append(out, b)
			continue
		}
		if lv, h, ok := heading(t); ok {
			flush()
			out = append(out, Block{Kind: Heading, Level: lv, Spans: Inline(h)})
			continue
		}
		if lv, ord, s, ok := listItem(raw); ok {
			start(Block{Kind: ListItem, Level: lv, Ordinal: ord}, s)
			continue
		}
		switch {
		case isFence(t):
			flush()
			info := strings.TrimSpace(strings.TrimPrefix(t, "```"))
			b := Block{Kind: CodeBlock, Info: info}
			if f := strings.Fields(info); len(f) > 0 {
				b.Lang = f[0]
			}
			for i++; i < len(lines) && !isFence(strings.TrimSpace(lines[i])); i++ {
				b.Lines = append(b.Lines, lines[i])
			}
			out = append(out, b)
		case t == "":
			flush()
		case isRule(t) && pend == nil:
			out = append(out, Block{Kind: Rule})
		case strings.HasPrefix(t, "|"):
			flush()
			b := Block{Kind: Table}
			for ; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|"); i++ {
				row := strings.TrimSpace(lines[i])
				if separatorRow(row) {
					b.Header = len(b.Rows) == 1
					continue
				}
				var cs [][]Span
				for _, c := range cells(row) {
					cs = append(cs, Inline(c))
				}
				b.Rows = append(b.Rows, cs)
			}
			i--
			out = append(out, b)
		case strings.HasPrefix(t, ">"):
			q := strings.TrimSpace(strings.TrimPrefix(t, ">"))
			if pend == nil || pend.Kind != Quote {
				start(Block{Kind: Quote}, q)
			} else {
				text = append(text, q)
			}
		case pend == nil:
			start(Block{Kind: Paragraph}, t)
		default:
			text = append(text, t)
		}
	}
	flush()
	return out
}
