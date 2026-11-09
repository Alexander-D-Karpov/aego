package main

import (
	"bytes"
	"os"
	"slices"
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

func TestIconsIncludeModuleToolbarUses(t *testing.T) {
	report, err := iconReport()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"render2d.normals":     "Scene View: surface normals",
		"render2d.point_light": "Scene View: lighting preview",
		"render2d.shadow_view": "Scene View: shadow visibility",
		"particles.system":     "Assets: New Effect command and Create palette",
	}
	for _, row := range report.Rows() {
		if by, ok := want[row.ID]; ok && slices.Contains(row.UsedBy, by) {
			delete(want, row.ID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("module toolbar icons missing from the audit: %v", want)
	}
}
