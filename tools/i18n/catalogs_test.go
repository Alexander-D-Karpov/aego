package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"aego/engine/i18n"
)

func TestExtractPreservesTranslationsAndCheckReportsDrift(t *testing.T) {
	root := t.TempDir()
	keys := map[string]*Key{
		"editor.greeting":   {ID: "editor.greeting", En: "Hello {name}"},
		"api.example.Value": {ID: "api.example.Value", En: "An example value."},
	}
	write := func(dir, lang, text string) {
		t.Helper()
		path := filepath.Join(root, dir, lang+".aetext")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const messagesRU = `messages version=1 lang=ru {
  msg "editor.greeting" { text="Привет, {name}" }
  msg "editor.retired" { text="Старый перевод" }
}
`
	const dataRU = "messages version=1 lang=ru {}\n"
	write(engineDir, "ru", messagesRU)
	write(dataDir, "ru", dataRU)
	if n, err := Extract(root, keys); err != nil || n != 2 {
		t.Fatalf("extract: %d catalogs, %v", n, err)
	}
	for dir, want := range map[string]string{engineDir: messagesRU, dataDir: dataRU} {
		got, err := os.ReadFile(filepath.Join(root, dir, "ru.aetext"))
		if err != nil || !bytes.Equal(got, []byte(want)) {
			t.Fatalf("extract changed %s/ru.aetext: %v", dir, err)
		}
	}
	check := func() *Report {
		t.Helper()
		r, err := Check(root, keys)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := check()
	if len(r.Stale) != 0 || len(r.Findings) != 2 || r.Missing[i18n.Russian] != 1 || !r.Failed(false) {
		t.Fatalf("missing and unused translations: %+v", r)
	}
	write(engineDir, "ru", `messages version=1 lang=ru { msg "editor.greeting" { text="Привет, {name}" } }`)
	r = check()
	if len(r.Findings) != 1 || r.Findings[0].Code != i18n.MissingKey || r.Failed(false) || !r.Failed(true) {
		t.Fatalf("missing translation in normal and strict mode: %+v", r)
	}
	write(dataDir, "ru", `messages version=1 lang=ru { msg "api.example.Value" { text="Пример значения." } }`)
	if r = check(); r.Failed(true) || len(r.Findings) != 0 {
		t.Fatalf("complete catalogs: %+v", r)
	}
	keys["api.example.Value"].En = "A changed example value."
	if r = check(); len(r.Stale) != 1 || r.Stale[0] != filepath.ToSlash(filepath.Join(dataDir, "en.aetext")) || !r.Failed(false) {
		t.Fatalf("changed English value: %+v", r)
	}
	write(engineDir, "ru", `messages version=1 lang=ru { msg "editor.greeting" { text="Привет, {who}" } }`)
	if r = check(); len(r.Findings) != 2 || r.Findings[0].Code != i18n.BadPlaceholder || r.Findings[1].Code != i18n.BadPlaceholder {
		t.Fatalf("changed placeholders: %+v", r)
	}
}
