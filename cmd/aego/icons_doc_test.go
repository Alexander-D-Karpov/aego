package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"aego/engine/module"
)

func TestIconsDocUpToDate(t *testing.T) {
	want, err := iconsMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../docs/icons.md")
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("docs/icons.md is stale, run make icons-doc")
	}
}

func TestIconsCheckReportsMissingEditorIcon(t *testing.T) {
	extra := append(editorIcons(), module.IconUse{ID: "ui.not_drawn", Owner: "editor", By: "test"})
	rep, err := builtinAudit(extra)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if rep.Write(&b) == 0 || !strings.Contains(b.String(), "ui.not_drawn") {
		t.Fatalf("a missing editor icon must fail the check:\n%s", b.String())
	}
}
