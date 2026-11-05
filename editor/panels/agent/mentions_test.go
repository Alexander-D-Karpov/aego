package agent

import (
	"strings"
	"testing"

	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/engine/base"
	"aego/platform/input"
	"aego/scene"
	"aego/ui/core"
)

func TestMentionTypedSearchPickedAndInsertedAtCaret(t *testing.T) {
	for _, tc := range []struct {
		name, query, label, full string
	}{
		{"node", "Hero", "@Hero", "@Hero (#"},
		{"asset", "walk", "@player_walk.aeanim", "@assets/anim/player_walk.aeanim"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			types, err := base.NewTypes()
			if err != nil {
				t.Fatal(err)
			}
			sc := scene.New(types, nil, scene.Options{})
			sc.Create(sc.Root(), "Hero")
			sc.Flush()
			mgr := document.NewManager(document.Options{Root: t.TempDir(), Types: types})
			doc := mgr.New("scenes/main.aescene", sc)
			host := newProviderTestHost()
			p := providerPanel(t, host)
			p.d.Docs = &quickDocs{doc: doc}
			p.d.Assets = func() []string { return []string{"assets/anim/player_walk.aeanim", "assets/anim/enemy.aectrl"} }
			p.Ask("end")
			ui, in := core.New(nil), &input.State{}
			r := mathx.Rect{W: 480, H: 520}
			draw := func() {
				ui.Begin(in, mathx.V2(800, 600), 1)
				ui.Region(r)
				p.Draw(&services.Frame{UI: ui}, r)
				ui.PopCursor()
				ui.End()
			}
			frame := func(event func()) {
				in.BeginFrame()
				if event != nil {
					event()
				}
				draw()
			}
			frame(nil)
			frame(func() { in.ApplyKeyDown(input.KeyHome, 0, 0, false) })
			frame(func() { in.ApplyKeyUp(input.KeyHome, 0, 0) })
			frame(func() { in.ApplyText([]byte("@")) })
			if p.pal == nil || !p.pal.Active() || p.input != "end" || ui.Focus() != p.pal.SearchID() {
				t.Fatalf("opening lost the draft or focus: input=%q focus=%v", p.input, ui.Focus())
			}
			frame(func() { in.ApplyText([]byte(tc.query)) })
			if p.pal.Query() != tc.query || p.input != "end" {
				t.Fatalf("search copied trigger or leaked into draft: query=%q input=%q", p.pal.Query(), p.input)
			}
			frame(func() { in.ApplyKeyDown(input.KeyEnter, 0, 0, false) })
			if p.pal.Active() || p.input != tc.label+" end" || len(p.picked) != 1 {
				t.Fatalf("selection not inserted at caret: %q, %#v", p.input, p.picked)
			}
			frame(func() { in.ApplyKeyUp(input.KeyEnter, 0, 0) })
			frame(func() { in.ApplyText([]byte("!")) })
			if p.input != tc.label+" !end" {
				t.Fatalf("typing did not resume at the caret: %q", p.input)
			}
			p.send()
			if len(host.sent) != 1 || !strings.Contains(host.sent[0], tc.full) {
				t.Fatalf("missing full reference in sent text: %q", host.sent)
			}
		})
	}
}

func TestMentionAssetIcons(t *testing.T) {
	for path, want := range map[string]string{
		"assets/walk.aeanim": "anim.key", "assets/player.aectrl": "anim.graph",
		"assets/scene.aescene": "file.scene", "assets/data.aetable": "file.table",
	} {
		if got := assetIcon(path); got != want {
			t.Errorf("assetIcon(%q) = %q, want %q", path, got, want)
		}
	}
}
