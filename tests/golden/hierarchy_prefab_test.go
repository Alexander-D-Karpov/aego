//go:build aego_golden

package golden

import (
	"fmt"
	"image"
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/prefabs"
	"aego/editor/core/selection"
	"aego/editor/core/services"
	"aego/editor/panels/hierarchy"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/script/bind"
	"aego/ui/dnd"
)

func TestEditorHierarchyPrefab(t *testing.T) {
	img, err := shotHierarchyPrefab(t, newEditorEnvSize(t, 320, 240))
	checkGolden(t, "editor_hierarchy_prefab", img, err)
}

func shotHierarchyPrefab(t *testing.T, env *editorEnv) (*image.NRGBA, error) {
	t.Helper()
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Default().Load(types, bind.NewManifest(), assembly.Options{}, prefabModule{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	reg := types.Schema()

	ps := scene.New(types, nil, scene.Options{})
	enemy := ps.Create(ps.Root(), "Enemy")
	ps.Create(enemy, "Stomp")
	scene.Add[goldenStats](ps, enemy).Speed = 2.5
	ps.Flush()
	model, err := sceneio.ParsePrefab(sceneio.SavePrefab(ps), "enemy.aeprefab", reg)
	if err != nil {
		t.Fatal(err)
	}
	pid := ids.AssetID{Hi: 0xe1, Lo: 0x01}
	lib := sceneio.NewLibrary(reg, nil)
	lib.Set(pid, model)
	rootKey := model.Local.String()

	s := scene.New(types, nil, scene.Options{})
	s.Create(s.Root(), "Ground")
	s.Flush()
	instance := func(node ids.NodeID, prefabID ids.AssetID, name, extra string) {
		t.Helper()
		block := fmt.Sprintf("prefab version=%d {\n  node %q prefab=asset(%q) {\n    id %q node=node(%q) {}\n    override %q prop=\"node.name\" value=%q {}\n%s  }\n}\n",
			sceneio.FormatVersion, node.String(), prefabID.String(), rootKey, node.String(), rootKey, name, extra)
		if _, _, err := sceneio.LoadInto(s, s.Root(), []byte(block), "golden", sceneio.LoadOptions{KeepIDs: true, Prefabs: lib}); err != nil {
			t.Fatal(err)
		}
		s.Flush()
	}
	left, right, lost := ids.NewNodeID(), ids.NewNodeID(), ids.NewNodeID()
	instance(left, pid, "EnemyLeft", "")
	instance(right, pid, "EnemyRight", fmt.Sprintf("    override %q prop=\"golden.Stats.speed\" value=1 {}\n", ids.NewNodeID().String()))
	instance(lost, ids.AssetID{Hi: 0xdead, Lo: 0x01}, "Lost", "")
	shield := s.Create(s.Handle(left), "Shield")
	s.Flush()
	stomp := s.ID(s.FirstChild(s.Handle(left)))

	ctx := &context.Editor{
		UI:        env.ui,
		Types:     types,
		Docs:      document.NewManager(document.Options{Root: t.TempDir(), Types: types, Log: log.Nop()}),
		Selection: selection.New(),
		Commands:  command.New(),
		DnD:       &dnd.State{},
		Settings:  context.DefaultSettings(),
		Collapsed: map[string]bool{},
	}
	ctx.Docs.New("scenes/prefabs.aescene", s)
	d := ctx.Doc()
	d.View.Expanded[left] = true
	d.View.Expanded[right] = true
	ctx.Select(s.ID(shield))

	svc := prefabs.New(ctx)
	svc.SetLibrary(lib)
	for _, c := range []struct {
		node   ids.NodeID
		status prefabs.Status
	}{
		{left, prefabs.StatusRoot},
		{stomp, prefabs.StatusInside},
		{s.ID(shield), prefabs.StatusAdded},
		{right, prefabs.StatusRoot},
		{lost, prefabs.StatusMissing},
	} {
		if got := svc.Row(d, c.node).Status; got != c.status {
			t.Fatalf("%s: status %d, want %d", s.Name(s.Handle(c.node)), got, c.status)
		}
	}
	if !svc.Row(d, right).Orphans {
		t.Fatal("EnemyRight must report its unused override")
	}

	p := hierarchy.New(hierarchy.Deps{
		Docs:      ctx,
		Selection: ctx,
		Commands:  ctx.Commands,
		Notices:   ctx,
		Prefabs:   svc,
	})
	img, err := env.draw(func(full mathx.Rect) { p.Draw(&services.Frame{UI: env.ui, DnD: ctx.DnD}, full) })
	return img, err
}
