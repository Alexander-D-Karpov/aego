package editor

import (
	"os"
	"path/filepath"
	"testing"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/render/material"
)

func TestShaderProblemOpensRealIncludeLine(t *testing.T) {
	s := &materialsEditor{}
	id := ids.AssetID{Lo: 1}
	err := errs.Wrap(errs.New(errs.AssetImportFailed, "unsupported declaration").With(log.F("code", "SHD004"), log.F("file", "assets/shaders/include/color.glsl"), log.F("line", 17)), errs.AssetImportFailed, "shader import")
	s.updateDiagnostics(id, "assets/shaders/a.aeshader", err)
	if len(s.diagnostics) != 1 {
		t.Fatal(s.diagnostics)
	}
	d := s.diagnostics[0]
	if d.Subject.File != "assets/shaders/include/color.glsl" || d.Subject.Line != 17 || d.Subject.Col != 0 || d.Code != "SHD004" {
		t.Fatalf("wrong source location: %+v", d)
	}
	s.updateDiagnostics(id, "assets/shaders/a.aeshader", nil)
	if len(s.diagnostics) != 0 {
		t.Fatal("fixed source kept an old problem")
	}
}

func TestInspectorReportsOnlyItsDirtyShaderDependencies(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"include.glsl", "unrelated.glsl"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("// saved\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	docs := document.NewManager(document.Options{Root: root})
	include, err := docs.OpenScript("include.glsl")
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err := docs.OpenScript("unrelated.glsl")
	if err != nil {
		t.Fatal(err)
	}
	s := &materialsEditor{ctx: &context.Editor{Docs: docs}}
	sh := &material.Shader{Files: []string{"include.glsl"}}
	unrelated.Code.Replace(0, 0, "// edit\n")
	if s.shaderDirty(sh) {
		t.Fatal("unrelated draft mislabeled this material")
	}
	include.Code.Replace(0, 0, "// edit\n")
	if !s.shaderDirty(sh) {
		t.Fatal("Inspector hid a current-source versus runtime difference in an include")
	}
	include.Code.Undo()
	if s.shaderDirty(sh) {
		t.Fatal("undo back to saved source must clear the source-state label")
	}
}

func TestNoGPUPreviewHasReason(t *testing.T) {
	p := materialPreview{}
	p.update(&context.Editor{}, &material.Material{}, 1, 1)
	if p.mat != nil || p.reason == "" {
		t.Fatal("headless session pretended to render a preview")
	}
}
