package integration

import (
	"testing"

	"aego/engine/res"
	"aego/gpu/null"
	"aego/render/icons"
	"aego/render/texture"
	"aego/vfs"
)

// The editor swallows an icon-atlas load failure into a warning and falls back
// to placeholder letters, so a broken embed, VFS path, or JSON shape would pass
// every other test silently. This loads the real embedded atlas end to end and
// asserts the ids the toolbar and inspector actually reference resolve.
func TestEmbeddedIconAtlasLoads(t *testing.T) {
	v, err := vfs.New(vfs.Mount{Scheme: "engine", Source: res.Source(), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	set, err := icons.Load(v, texture.NewUploader(null.New()), "engine://icons")
	if err != nil {
		t.Fatalf("embedded icon atlas failed to load: %v", err)
	}
	for _, id := range []string{
		"tool.select", "tool.move", "tool.rotate", "tool.scale",
		"view.grid", "view.snap", "view.zoom",
		"play.start", "play.pause", "play.step", "play.stop",
		"file.save", "cat.node", "cat.transform",
		"ui.search", "ui.more", "build.ok", "build.busy", "build.error",
	} {
		if !set.Has(id) {
			t.Fatalf("icon %q missing from the embedded atlas", id)
		}
		if _, ok := set.Region(id, 16, 1); !ok {
			t.Fatalf("icon %q did not resolve a 16px region", id)
		}
		if _, ok := set.Region(id, 24, 2); !ok {
			t.Fatalf("icon %q did not resolve a 24px@2x region", id)
		}
	}
}
