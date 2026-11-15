package app

import (
	"path/filepath"
	"testing"

	"aego/app/docicons"
	"aego/core/log"
	"aego/engine"
	"aego/project"
)

func TestEditorHostInstallsDocumentationIconPolicy(t *testing.T) {
	p, err := project.Create(filepath.Join(t.TempDir(), "project"), "Docs icons", false)
	if err != nil {
		t.Fatal(err)
	}
	h, err := Build(p, Config{Engine: engine.Config{Headless: true}, Logger: log.Nop(), Docs: true, Editor: true, NoProfile: true})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if h.Docs == nil {
		t.Fatal("editor host did not build its documentation registry")
	}
	tasks, refs := docicons.Builtin()
	for id, want := range tasks {
		if got := h.Docs.NavigationTaskIcon(id); got != want {
			t.Errorf("host task %q = %q, want %q", id, got, want)
		}
	}
	for id, want := range refs {
		if got := h.Docs.NavigationReferenceIcon(id); got != want {
			t.Errorf("host reference %q = %q, want %q", id, got, want)
		}
	}
	for _, id := range []string{"animation", "animation/animator", "render2d.PostProcess", "camera.CameraBrain2D", "aego/audio", "aego/timer", "section/animation"} {
		if _, ok := h.Docs.Get(id); !ok {
			t.Errorf("editor host lacks module documentation entry %q", id)
		}
	}
}
