// Package i18n translates the editor, the launcher and the CLI; English lives in the code next to each key (D348).
package i18n

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"aego/format/aetext"
)

// Languages the editor ships catalogs for; "auto" picks one from the OS.
const (
	English = "en"
	Russian = "ru"
	Auto    = "auto"
)

// Diagnostic codes of the translation layer, reported in dev mode.
const (
	MissingKey     = "I18N001"
	UnusedKey      = "I18N002"
	BadPlaceholder = "I18N003"
)

// Form is a CLDR plural category.
type Form uint8

const (
	Other Form = iota
	One
	Few
	Many
)

var formNames = [...]string{"other", "one", "few", "many"}

func (f Form) String() string { return formNames[f] }

// Message is one translated entry: a plain text, or one text per plural form.
type Message struct {
	Text    string
	Plural  map[Form]string
	Context string
}

// Catalog holds the messages of one language.
type Catalog struct {
	Lang string
	Msgs map[string]Message
	// Order keeps the keys in file order for tools that rewrite catalogs.
	Order []string
}

// Parse reads a catalog: messages version=1 lang=ru { msg "key" { text="…" } msg "n" { one="…" few="…" many="…" } }.
func Parse(data []byte, name string) (*Catalog, error) {
	doc, err := aetext.Parse(data, name)
	if err != nil {
		return nil, err
	}
	if doc.Head != "messages" {
		return nil, fmt.Errorf("%s: want a messages block, got %q", name, doc.Head)
	}
	lang, _ := doc.Attr("lang")
	c := &Catalog{Lang: lang.Str, Msgs: map[string]Message{}}
	if c.Lang == "" {
		c.Lang = lang.Ident
	}
	if c.Lang == "" {
		return nil, fmt.Errorf("%s: messages has no lang", name)
	}
	for _, b := range doc.Blocks {
		if b.Head != "msg" {
			continue
		}
		if b.Label == "" {
			return nil, fmt.Errorf("%s:%d: msg without a key", name, b.Pos.Line)
		}
		m := Message{}
		for _, f := range b.Fields {
			switch f.Name {
			case "text":
				m.Text = f.Value.Str
			case "context":
				m.Context = f.Value.Str
			case "one", "few", "many", "other":
				if m.Plural == nil {
					m.Plural = map[Form]string{}
				}
				m.Plural[formOf(f.Name)] = f.Value.Str
			default:
				return nil, fmt.Errorf("%s:%d: unknown field %q in msg %q", name, f.Pos.Line, f.Name, b.Label)
			}
		}
		if _, dup := c.Msgs[b.Label]; dup {
			return nil, fmt.Errorf("%s:%d: msg %q declared twice", name, b.Pos.Line, b.Label)
		}
		c.Msgs[b.Label] = m
		c.Order = append(c.Order, b.Label)
	}
	return c, nil
}

func formOf(s string) Form {
	for i, n := range formNames {
		if n == s {
			return Form(i)
		}
	}
	return Other
}

// Write renders a catalog in key order.
func Write(c *Catalog) []byte {
	root := &aetext.Node{Head: "messages", Attrs: []aetext.Field{{Name: "version", Value: aetext.Int(1)}, {Name: "lang", Value: aetext.Ident(c.Lang)}}}
	keys := c.Order
	if len(keys) != len(c.Msgs) {
		keys = make([]string, 0, len(c.Msgs))
		for k := range c.Msgs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}
	for _, k := range keys {
		m := c.Msgs[k]
		n := &aetext.Node{Head: "msg", Label: k}
		if m.Context != "" {
			n.Fields = append(n.Fields, aetext.Field{Name: "context", Value: aetext.String(m.Context)})
		}
		if m.Plural == nil {
			n.Fields = append(n.Fields, aetext.Field{Name: "text", Value: aetext.String(m.Text)})
		}
		for _, f := range []Form{One, Few, Many, Other} {
			if v, ok := m.Plural[f]; ok {
				n.Fields = append(n.Fields, aetext.Field{Name: f.String(), Value: aetext.String(v)})
			}
		}
		root.Blocks = append(root.Blocks, n)
	}
	return aetext.Write(root)
}

// PluralForm is the CLDR category of an integer count in a language.
func PluralForm(lang string, n int64) Form {
	if n < 0 {
		n = -n
	}
	switch lang {
	case Russian:
		switch m10, m100 := n%10, n%100; {
		case m10 == 1 && m100 != 11:
			return One
		case m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14):
			return Few
		default:
			return Many
		}
	}
	if n == 1 {
		return One
	}
	return Other
}

type argKind uint8

const (
	argStr argKind = iota
	argInt
	argFloat
)

// Arg is a typed value for a {name} placeholder.
type Arg struct {
	name string
	kind argKind
	s    string
	i    int64
	f    float64
	prec int
}

// Args is the list of placeholder values of one message.
type Args []Arg

func Str(name, v string) Arg       { return Arg{name: name, kind: argStr, s: v} }
func Int(name string, v int64) Arg { return Arg{name: name, kind: argInt, i: v} }
func Float(name string, v float64, prec int) Arg {
	return Arg{name: name, kind: argFloat, f: v, prec: prec}
}

func (a Arg) text() string {
	switch a.kind {
	case argInt:
		return strconv.FormatInt(a.i, 10)
	case argFloat:
		return strconv.FormatFloat(a.f, 'f', a.prec, 64)
	}
	return a.s
}

// Problem is a translation diagnostic seen at run time, in dev mode.
type Problem struct {
	Code    string
	Key     string
	Message string
}

// Bundle holds the catalogs of every language and the language in use.
type Bundle struct {
	mu       sync.Mutex
	cats     map[string]*Catalog
	lang     atomic.Pointer[string]
	cur      atomic.Pointer[map[string]Message]
	dev      atomic.Bool
	seen     map[string]bool
	loaded   map[string]bool
	problems []Problem
	used     map[string]bool
}

func NewBundle() *Bundle {
	b := &Bundle{cats: map[string]*Catalog{}, seen: map[string]bool{}, used: map[string]bool{}, loaded: map[string]bool{}}
	en := English
	b.lang.Store(&en)
	return b
}

//go:embed messages/*.aetext data/*.aetext
var messages embed.FS

var def = engineBundle()

func engineBundle() *Bundle {
	b := NewBundle()
	if err := b.LoadOnce("engine", messages, "messages", ""); err != nil {
		panic("i18n: " + err.Error())
	}
	if err := b.LoadOnce("engine data", messages, "data", ""); err != nil {
		panic("i18n: " + err.Error())
	}
	return b
}

// Default is the bundle the editor, the launcher and the CLI translate through.
func Default() *Bundle { return def }

// Add merges a catalog; a key the bundle already has keeps its first text, a key outside namespace is an error.
func (b *Bundle) Add(c *Catalog, namespace string) error {
	if namespace != "" {
		for _, k := range c.Order {
			if !strings.HasPrefix(k, namespace+".") {
				return fmt.Errorf("%s catalog: key %q is outside %s.*", c.Lang, k, namespace)
			}
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	dst := b.cats[c.Lang]
	if dst == nil {
		dst = &Catalog{Lang: c.Lang, Msgs: map[string]Message{}}
		b.cats[c.Lang] = dst
	}
	msgs := make(map[string]Message, len(dst.Msgs)+len(c.Msgs))
	for k, m := range dst.Msgs {
		msgs[k] = m
	}
	for _, k := range c.Order {
		if _, dup := msgs[k]; dup {
			continue
		}
		msgs[k] = c.Msgs[k]
		dst.Order = append(dst.Order, k)
	}
	dst.Msgs = msgs
	if *b.lang.Load() == c.Lang {
		b.cur.Store(&msgs)
	}
	return nil
}

// Load reads every <lang>.aetext of a directory into the bundle.
func (b *Bundle) Load(fsys fs.FS, dir, namespace string) error {
	files, err := fs.Glob(fsys, path.Join(dir, "*.aetext"))
	if err != nil {
		return err
	}
	for _, f := range files {
		data, err := fs.ReadFile(fsys, f)
		if err != nil {
			return err
		}
		c, err := Parse(data, f)
		if err != nil {
			return err
		}
		if err := b.Add(c, namespace); err != nil {
			return err
		}
	}
	return nil
}

// LoadOnce is Load for a source named owner, skipped when that owner is already loaded.
func (b *Bundle) LoadOnce(owner string, fsys fs.FS, dir, namespace string) error {
	b.mu.Lock()
	done := b.loaded[owner]
	b.loaded[owner] = true
	b.mu.Unlock()
	if done || fsys == nil {
		return nil
	}
	return b.Load(fsys, dir, namespace)
}

// SetLanguage switches every following Tr; an unknown language uses English.
func (b *Bundle) SetLanguage(lang string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lang.Store(&lang)
	if c := b.cats[lang]; c != nil && lang != English {
		b.cur.Store(&c.Msgs)
	} else {
		b.cur.Store(nil)
	}
}

func (b *Bundle) Language() string { return *b.lang.Load() }

// SetDev turns on the run-time diagnostics.
func (b *Bundle) SetDev(on bool) { b.dev.Store(on) }

// Languages lists the languages with a catalog, English first.
func (b *Bundle) Languages() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []string{English}
	for l := range b.cats {
		if l != English {
			out = append(out, l)
		}
	}
	sort.Strings(out[1:])
	return out
}

// Catalog returns the merged catalog of a language.
func (b *Bundle) Catalog(lang string) *Catalog {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cats[lang]
}

// Translations returns loaded labels for search without switching the UI
// language or exposing a mutable catalog. Output order is deterministic.
func (b *Bundle) Translations(key string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, catalog := range b.cats {
		if text := catalog.Msgs[key].Text; text != "" {
			out = append(out, text)
		}
	}
	sort.Strings(out)
	return out
}

func (b *Bundle) lookup(key string) (Message, bool) {
	msgs := b.cur.Load()
	if msgs == nil {
		return Message{}, false
	}
	m, ok := (*msgs)[key]
	if b.dev.Load() {
		b.mu.Lock()
		b.used[key] = true
		if !ok && !b.seen[key] {
			b.seen[key] = true
			b.problems = append(b.problems, Problem{Code: MissingKey, Key: key, Message: "no " + *b.lang.Load() + " text for " + key})
		}
		b.mu.Unlock()
	}
	return m, ok
}

// Tr is the text of key in the current language, or en.
func (b *Bundle) Tr(key, en string, args ...Arg) string {
	text := en
	if m, ok := b.lookup(key); ok && m.Text != "" {
		text = m.Text
	}
	if len(args) == 0 && strings.IndexByte(text, '{') < 0 {
		return text
	}
	return b.fill(key, text, args)
}

// TrN picks the plural form of n; {n} in the text is the count.
func (b *Bundle) TrN(key string, n int64, one, other string, args ...Arg) string {
	text := other
	if PluralForm(English, n) == One {
		text = one
	}
	lang := b.Language()
	if m, ok := b.lookup(key); ok && m.Plural != nil {
		f := PluralForm(lang, n)
		if t, ok := m.Plural[f]; ok {
			text = t
		} else if t, ok := m.Plural[Other]; ok {
			text = t
		}
	}
	return b.fill(key, text, append(args, Int("n", n)))
}

func (b *Bundle) fill(key, text string, args Args) string {
	var sb strings.Builder
	for {
		i := strings.IndexByte(text, '{')
		if i < 0 {
			sb.WriteString(text)
			return sb.String()
		}
		j := strings.IndexByte(text[i:], '}')
		if j < 0 {
			sb.WriteString(text)
			return sb.String()
		}
		name := text[i+1 : i+j]
		sb.WriteString(text[:i])
		if !isName(name) {
			sb.WriteByte('{')
			text = text[i+1:]
			continue
		}
		found := false
		for _, a := range args {
			if a.name == name {
				sb.WriteString(a.text())
				found = true
				break
			}
		}
		if !found {
			sb.WriteString(text[i : i+j+1])
			b.report(BadPlaceholder, key, "placeholder {"+name+"} has no value")
		}
		text = text[i+j+1:]
	}
}

func (b *Bundle) report(code, key, msg string) {
	if !b.dev.Load() {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.seen[code+key] {
		return
	}
	b.seen[code+key] = true
	b.problems = append(b.problems, Problem{Code: code, Key: key, Message: msg})
}

// Problems are the run-time diagnostics so far.
func (b *Bundle) Problems() []Problem {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Problem(nil), b.problems...)
}

// TakeProblems returns the run-time diagnostics and forgets them, so each session reports its own.
func (b *Bundle) TakeProblems() []Problem {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.problems
	b.problems = nil
	return out
}

// Unused lists the keys of a language that no Tr asked for while dev mode was on.
func (b *Bundle) Unused(lang string) []Problem {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []Problem
	if c := b.cats[lang]; c != nil {
		for _, k := range c.Order {
			if !b.used[k] {
				out = append(out, Problem{Code: UnusedKey, Key: k, Message: k + " is never used"})
			}
		}
	}
	return out
}

// Placeholders lists the {names} of a text, for checking a translation against its English source.
func Placeholders(text string) []string {
	var out []string
	for {
		i := strings.IndexByte(text, '{')
		if i < 0 {
			return out
		}
		j := strings.IndexByte(text[i:], '}')
		if j < 0 {
			return out
		}
		if name := text[i+1 : i+j]; isName(name) {
			out = append(out, name)
			text = text[i+j+1:]
		} else {
			text = text[i+1:]
		}
	}
}

func isName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// Tr translates through the default bundle.
func Tr(key, en string, args ...Arg) string { return def.Tr(key, en, args...) }

// TrN translates a count through the default bundle.
func TrN(key string, n int64, one, other string, args ...Arg) string {
	return def.TrN(key, n, one, other, args...)
}

// Resolve is the language for a preference: en or ru as given, or for auto the first OS locale with a catalog.
func Resolve(pref string, locales, have []string) string {
	if pref != Auto && pref != "" {
		return pref
	}
	for _, l := range locales {
		lang, _, _ := strings.Cut(strings.ToLower(strings.ReplaceAll(l, "_", "-")), "-")
		for _, h := range have {
			if h == lang {
				return h
			}
		}
	}
	return English
}

// Text is a key with its English text, declared where a label is defined and translated where it is drawn.
type Text struct {
	Key string
	En  string
}

// T declares a translatable text; tools/i18n extract reads its literals.
func T(key, en string) Text { return Text{Key: key, En: en} }

// String is the text in the current language; a Text with no key is its English as is.
func (t Text) String() string {
	if t.Key == "" {
		return t.En
	}
	return def.Tr(t.Key, t.En)
}

// Lookup translates a key built at run time; tools/i18n does not extract it, so its key must come from a literal elsewhere.
func Lookup(key, en string) string { return def.Tr(key, en) }

// Apply sets the default bundle's language for a preference; AEGO_LANG overrides it, and auto asks locales.
func Apply(pref string, locales func() []string) string {
	if v := os.Getenv("AEGO_LANG"); v != "" {
		pref = v
	}
	var ls []string
	if (pref == Auto || pref == "") && locales != nil {
		ls = locales()
	}
	lang := Resolve(pref, ls, def.Languages())
	def.SetLanguage(lang)
	return lang
}

var notes struct {
	mu sync.Mutex
	en map[string]string
}

// Note is Tr for a message that is also logged: the log keeps English, so EnglishOf maps the shown text back.
func Note(key, en string, args ...Arg) string {
	t := def.Tr(key, en, args...)
	if def.Language() == English {
		return t
	}
	e := en
	if len(args) > 0 || strings.IndexByte(en, '{') >= 0 {
		e = def.fill(key, en, args)
	}
	notes.mu.Lock()
	if notes.en == nil || len(notes.en) > 256 {
		notes.en = map[string]string{}
	}
	notes.en[t] = e
	notes.mu.Unlock()
	return t
}

// EnglishOf is the English text of a message made by Note, or the text itself.
func EnglishOf(text string) string {
	notes.mu.Lock()
	defer notes.mu.Unlock()
	if e, ok := notes.en[text]; ok {
		return e
	}
	return text
}

// In is the text of key in lang, or en, without switching the bundle's language.
func (b *Bundle) In(lang, key, en string) string {
	if lang == English || lang == "" {
		return en
	}
	b.mu.Lock()
	c := b.cats[lang]
	b.mu.Unlock()
	if c != nil {
		if m, ok := c.Msgs[key]; ok && m.Text != "" {
			return m.Text
		}
	}
	return en
}
