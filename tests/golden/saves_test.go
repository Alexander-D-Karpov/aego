//go:build aego_golden

package golden

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/services"
	"aego/editor/panels/saves"
	"aego/format/aetext"
	"aego/save"
	"aego/ui/dnd"
)

func TestEditorSaves(t *testing.T) {
	local := time.Local
	time.Local = time.UTC
	defer func() { time.Local = local }()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	env := newEditorEnv(t)
	st := save.NewStore("saves")
	mod := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	scene := ids.AssetID{Hi: 0x7a1e, Lo: 1}
	write := func(slot string, playtime float64, spawned int) {
		d := &save.Doc{Meta: save.Meta{Name: slot, Game: "mygame@0.1.0", Engine: "test", Scene: scene,
			Date: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC), Playtime: playtime, Objects: 2 + spawned, Counter: uint64(spawned)}}
		d.Nodes = append(d.Nodes, &aetext.Node{Head: "node", Label: ids.NodeID{Hi: 1, Lo: 1}.String()}, &aetext.Node{Head: "node", Label: ids.NodeID{Hi: 1, Lo: 2}.String()})
		for i := 0; i < spawned; i++ {
			d.Spawned = append(d.Spawned, &aetext.Node{Head: "spawned", Label: save.PersistentID(uint64(i + 1)).String()})
		}
		d.Global = []aetext.Field{{Name: "kills", Value: aetext.Int(4)}}
		if err := st.Write(slot, save.Encode(d), nil); err != nil {
			t.Fatal(err)
		}
		mod = mod.Add(time.Minute)
		if err := os.Chtimes(filepath.Join("saves", slot+save.Ext), mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	write("quick", 754, 3)
	write("checkpoint", 3725, 0)
	p := saves.New(saves.Deps{
		Store: func() *save.Store { return st },
		Label: func(ids.AssetID) string { return "scenes/main.aescene" },
	})
	p.Select("quick")
	img, err := env.draw(func(full mathx.Rect) {
		p.Draw(&services.Frame{UI: env.ui, DnD: &dnd.State{}, Now: 100}, full)
	})
	if cerr := os.Chdir(wd); cerr != nil {
		t.Fatal(cerr)
	}
	checkGolden(t, "editor_saves", img, err)
}
