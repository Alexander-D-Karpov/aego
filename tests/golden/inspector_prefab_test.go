//go:build aego_golden

package golden

import (
	"fmt"
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/clipboard"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/prefabs"
	"aego/editor/core/selection"
	"aego/editor/core/services"
	"aego/editor/ext/inspect"
	"aego/editor/panels/inspector"
	"aego/engine/module"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/script/bind"
	"aego/ui/dnd"
)

type goldenStats struct {
	Speed float32     `aego:"speed"`
	Tint  mathx.Color `aego:"tint"`
}

type prefabModule struct{}

func (prefabModule) Info() module.Info {
	return module.Info{ID: "golden"}
}

func (prefabModule) Register(r *module.Registry) error {
	return module.Component[goldenStats](r, schema.Name("golden.Stats")).Err()
}

func TestEditorInspectorPrefab(t *testing.T) {
	env := newEditorEnv(t)
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
	st := scene.Add[goldenStats](ps, enemy)
	st.Speed = 2.5
	st.Tint = mathx.Color{R: 0.88, G: 0.34, B: 0.23, A: 1}
	ps.Flush()
	model, err := sceneio.ParsePrefab(sceneio.SavePrefab(ps), "enemy.aeprefab", reg)
	if err != nil {
		t.Fatal(err)
	}
	pid := ids.AssetID{Hi: 0xe1, Lo: 0x01}
	lib := sceneio.NewLibrary(reg, nil)
	lib.Set(pid, model)

	s := scene.New(types, nil, scene.Options{})
	rootID := ids.NewNodeID()
	block := fmt.Sprintf("prefab version=%d {\n  node %q prefab=asset(%q) {\n    override %q prop=\"golden.Stats.speed\" value=1 {}\n  }\n}\n",
		sceneio.FormatVersion, rootID.String(), pid.String(), ids.NewNodeID().String())
	if _, _, err := sceneio.LoadInto(s, s.Root(), []byte(block), "golden", sceneio.LoadOptions{KeepIDs: true, Prefabs: lib}); err != nil {
		t.Fatal(err)
	}
	s.Flush()
	root := s.Handle(rootID)
	live, ok := scene.Get[goldenStats](s, root)
	if !ok {
		t.Fatal("instance did not expand")
	}
	live.Speed = 4

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
	ctx.Docs.New("scenes/prefab.aescene", s)
	ctx.Select(rootID)
	svc := prefabs.New(ctx)
	svc.SetLibrary(lib)
	info, ok := svc.Info(ctx.Doc(), rootID)
	if !ok || !info.Overridden(rootID, "golden.Stats", "speed") || info.Orphans().Empty() {
		t.Fatal("fixture must have a live override and an unused one")
	}

	ir := inspect.NewRegistry()
	if err := ir.Load(reg); err != nil {
		t.Fatal(err)
	}
	ir.SetEditor(ctx)
	p := inspector.New(ir, inspector.Deps{
		Docs:      ctx,
		Selection: ctx,
		Commands:  ctx.Commands,
		Notices:   ctx,
		Nav:       ctx,
		Problems:  ctx,
		Prefabs:   svc,
		Clip:      clipboard.EnvOf(ctx),
		Collapsed: ctx.Collapsed,
	})
	img, err := env.draw(func(full mathx.Rect) { p.Draw(&services.Frame{UI: env.ui, DnD: ctx.DnD}, full) })
	checkGolden(t, "editor_inspector_prefab", img, err)
}
