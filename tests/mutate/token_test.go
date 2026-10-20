//go:build aego_editor

package mutate_test

import (
	"bytes"
	"testing"

	"aego/core/ids"
	"aego/editor/core/document"
	"aego/editor/core/scenediff"
	"aego/format/aetext"
	"aego/project/refactor"
	"aego/project/refs"
)

func TestRefTokenMatchesSceneText(t *testing.T) {
	for _, id := range []ids.AssetID{{}, {Hi: 0x7a1e0c5b3d9f4a2e, Lo: 0x8c6b1d0f9e8a7c02}} {
		want := []byte(scenediff.ValueText(aetext.AssetRef(id.String())))
		if got := refs.Token(id); !bytes.Equal(got, want) {
			t.Fatalf("token %q, scene text %q", got, want)
		}
	}
	if refactor.ScriptExt != document.ScriptExt {
		t.Fatalf("script extension %q in refactor, %q in the editor", refactor.ScriptExt, document.ScriptExt)
	}
}
