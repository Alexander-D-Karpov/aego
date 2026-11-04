package sceneview

import (
	"bytes"
	"testing"

	"aego/core/ids"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/render/lighting"
	"aego/scene"
	"aego/scene/sceneio"
)

func TestLightingViewIsPerDocumentAndDoesNotDirtyScene(t *testing.T) {
	types := testTypes(t)
	docs := document.NewManager(document.Options{Root: t.TempDir(), Types: types})
	first := scene.New(types, nil, scene.Options{})
	docs.New("first.aescene", first)
	ed := &context.Editor{Docs: docs}
	p := New(nil, Deps{Docs: ed, Viewport: &services.Viewport{}})
	before := sceneio.Save(first)
	a := p.LightingOutput()
	p.SetLightingView(false, lighting.DebugNormals)
	p.SetLightingEnvironmentPreview(ids.NodeID{Lo: 719})
	if enabled, mode := p.LightingView(); enabled || mode != lighting.DebugNormals {
		t.Fatal("view mode not applied")
	}
	if !bytes.Equal(before, sceneio.Save(first)) || ed.Doc().History.Cursor() != 0 {
		t.Fatal("view change wrote scene")
	}
	docs.New("second.aescene", scene.New(types, nil, scene.Options{}))
	b := p.LightingOutput()
	if a == b {
		t.Fatal("two Scene Views share output identity")
	}
	if enabled, mode := p.LightingView(); !enabled || mode != lighting.DebugLighting {
		t.Fatal("new document inherited another view's debug mode")
	}
	if !p.LightingEnvironmentPreview().IsZero() {
		t.Fatal("new document inherited another view's environment")
	}
}
