package i18n

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestRussianPlurals(t *testing.T) {
	want := map[int64]Form{0: Many, 1: One, 2: Few, 5: Many, 11: Many, 21: One, 22: Few, 25: Many, 101: One, 111: Many, 12: Many, 14: Many, 104: Few}
	for n, f := range want {
		if got := PluralForm(Russian, n); got != f {
			t.Errorf("ru %d: %v, want %v", n, got, f)
		}
	}
	if PluralForm(English, 1) != One || PluralForm(English, 0) != Other || PluralForm(English, 2) != Other {
		t.Error("English plurals are wrong")
	}
}

const ru = `messages version=1 lang=ru {
  msg "editor.component.add" { text="Добавить компонент" }
  msg "editor.selection.count" { one="Выбран {n} узел" few="Выбрано {n} узла" many="Выбрано {n} узлов" }
  msg "editor.open" { text="Открыть {name}" }
  msg "editor.bad" { text="Открыть {file}" }
}
`

func bundle(t *testing.T) *Bundle {
	t.Helper()
	b := NewBundle()
	if err := b.Load(fstest.MapFS{"i18n/ru.aetext": {Data: []byte(ru)}}, "i18n", ""); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTranslateAndFallback(t *testing.T) {
	b := bundle(t)
	if got := b.Tr("editor.component.add", "Add Component"); got != "Add Component" {
		t.Fatalf("English: %q", got)
	}
	b.SetLanguage(Russian)
	b.SetDev(true)
	if got := b.Tr("editor.component.add", "Add Component"); got != "Добавить компонент" {
		t.Fatalf("Russian: %q", got)
	}
	if got := b.Tr("editor.missing", "Missing"); got != "Missing" {
		t.Fatalf("fallback: %q", got)
	}
	for _, n := range []int64{1, 3, 5, 21} {
		got := b.TrN("editor.selection.count", n, "{n} node selected", "{n} nodes selected")
		if !strings.Contains(got, "Выбра") {
			t.Fatalf("plural %d: %q", n, got)
		}
	}
	if got := b.TrN("editor.selection.count", 22, "", ""); got != "Выбрано 22 узла" {
		t.Fatalf("few: %q", got)
	}
	if got := b.Tr("editor.open", "Open {name}", Str("name", "main.aescene")); got != "Открыть main.aescene" {
		t.Fatalf("placeholder: %q", got)
	}
	b.Tr("editor.bad", "Open {name}", Str("name", "x"))
	codes := map[string]bool{}
	for _, p := range b.Problems() {
		codes[p.Code] = true
	}
	if !codes[MissingKey] || !codes[BadPlaceholder] {
		t.Fatalf("problems %v, want I18N001 and I18N003", b.Problems())
	}
	unused := b.Unused(Russian)
	if len(unused) != 0 {
		t.Fatalf("every key was used: %v", unused)
	}
}

func TestEnglishNoAlloc(t *testing.T) {
	b := bundle(t)
	if n := testing.AllocsPerRun(100, func() { b.Tr("editor.component.add", "Add Component") }); n != 0 {
		t.Fatalf("English Tr allocates %v", n)
	}
	b.SetLanguage(Russian)
	if n := testing.AllocsPerRun(100, func() { b.Tr("editor.component.add", "Add Component") }); n != 0 {
		t.Fatalf("Russian Tr allocates %v", n)
	}
}

func TestModuleNamespace(t *testing.T) {
	b := NewBundle()
	fs := fstest.MapFS{"i18n/ru.aetext": {Data: []byte("messages version=1 lang=ru {\n  msg \"i18n.audio.mixer\" { text=\"Микшер\" }\n  msg \"editor.stray\" { text=\"x\" }\n}\n")}}
	if err := b.Load(fs, "i18n", "i18n.audio"); err == nil || !strings.Contains(err.Error(), "editor.stray") {
		t.Fatalf("a key outside the namespace loads: %v", err)
	}
	fs["i18n/ru.aetext"] = &fstest.MapFile{Data: []byte("messages version=1 lang=ru {\n  msg \"i18n.audio.mixer\" { text=\"Микшер\" }\n}\n")}
	if err := b.Load(fs, "i18n", "i18n.audio"); err != nil {
		t.Fatal(err)
	}
	b.SetLanguage(Russian)
	if got := b.Tr("i18n.audio.mixer", "Mixer"); got != "Микшер" {
		t.Fatalf("module key: %q", got)
	}
}

func TestLiteralBraces(t *testing.T) {
	b := NewBundle()
	b.SetDev(true)
	if got := b.Tr("editor.x", "Write { } or {1}, not {name}", Str("name", "n")); got != "Write { } or {1}, not n" {
		t.Fatalf("braces: %q", got)
	}
	if got := Placeholders("{a} { } {b1} {2}"); len(got) != 2 || got[1] != "b1" {
		t.Fatalf("placeholders: %v", got)
	}
	if len(b.Problems()) != 0 {
		t.Fatalf("literal braces are problems: %v", b.Problems())
	}
}

func TestResolve(t *testing.T) {
	have := []string{English, Russian}
	for _, c := range []struct {
		pref, want string
		locs       []string
	}{
		{Auto, Russian, []string{"ru-RU", "en"}}, {Auto, English, []string{"de-DE"}}, {Auto, Russian, []string{"de", "ru_UA"}},
		{English, English, []string{"ru"}}, {Russian, Russian, nil}, {Auto, English, nil},
	} {
		if got := Resolve(c.pref, c.locs, have); got != c.want {
			t.Errorf("Resolve(%s, %v) = %s, want %s", c.pref, c.locs, got, c.want)
		}
	}
}

func TestWriteRoundTrip(t *testing.T) {
	c, err := Parse([]byte(ru), "ru.aetext")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := Parse(Write(c), "again")
	if err != nil || len(c2.Msgs) != len(c.Msgs) || c2.Msgs["editor.selection.count"].Plural[Few] != "Выбрано {n} узла" {
		t.Fatalf("round trip: %v %v", err, c2)
	}
}

// A notice shows the editor language and logs English.
func TestNoteLogsEnglish(t *testing.T) {
	def.SetLanguage(Russian)
	defer def.SetLanguage(English)
	shown := Note("editor.problems.no_problems", "No problems")
	if shown == "No problems" {
		t.Skip("no ru text for the key")
	}
	if got := EnglishOf(shown); got != "No problems" {
		t.Fatalf("EnglishOf(%q) = %q", shown, got)
	}
	if got := EnglishOf("unrelated"); got != "unrelated" {
		t.Fatalf("EnglishOf passes other text through: %q", got)
	}
}
