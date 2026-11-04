package context

import (
	"testing"

	"aego/core/ids"
	"aego/engine/problems"
)

func TestSourceImportExplainsOnlyMatchingPreviewWarning(t *testing.T) {
	asset := ids.AssetID{Lo: 7}
	source := problems.Diagnostic{
		Owner: "render2d", Code: "SHD008", Severity: problems.Error, Message: "shader import failed",
		Subject: problems.Subject{Kind: problems.SubjectFile, File: "assets/cold.aeshader", Asset: asset, Line: 10},
	}
	mirrored := source
	mirrored.Owner, mirrored.Code, mirrored.Severity = "editor.preview", "THUMB001", problems.Warning
	mirrored.Subject.Kind, mirrored.Subject.Line = problems.SubjectAsset, 0
	unrelated := mirrored
	unrelated.Message = "preview framebuffer unavailable"
	otherAsset := mirrored
	otherAsset.Subject.Asset = ids.AssetID{Lo: 8}
	otherAsset.Subject.File = "assets/other.aeshader"
	ed := &Editor{Problems: problems.New()}
	// These rows normally come from independent providers; both previews of
	// the same asset also need distinct subjects for this aggregate test.
	unrelated.Subject.Field = "preview-target"
	diagnostics := []problems.Diagnostic{source, mirrored, unrelated, otherAsset}
	ed.Problems.Add(problems.Source{Append: func(dst []problems.Diagnostic) []problems.Diagnostic { return append(dst, diagnostics...) }})
	rows, _ := ed.ScriptProblems(nil)
	if len(rows) != 3 {
		t.Fatalf("the source should cover only its forwarded error: %+v", rows)
	}
	if rows[0].Code != "SHD008" || rows[0].Line != 10 || rows[1].Message != unrelated.Message || rows[2].Path != otherAsset.Subject.File {
		t.Fatalf("source location or independent preview error lost: %+v", rows)
	}
	diagnostics = diagnostics[1:]
	rows, _ = ed.ScriptProblems(rows[:0])
	if len(rows) != 3 || rows[0].Code != "THUMB001" || rows[0].Message != mirrored.Message {
		t.Fatalf("standalone preview explanation was lost: %+v", rows)
	}
}
