//go:build aego_golden

package golden

import (
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/command"
	"aego/editor/context"
	"aego/editor/document"
	"aego/editor/inspector"
	"aego/editor/selection"
	"aego/engine/builtin"
	"aego/engine/module"
	"aego/scene"
	"aego/schema"
	"aego/script/bind"
	"aego/script/stdlib"
	"aego/ui/dnd"
)

type goldenRefs struct {
	None    scene.NodeRef            `aego:"none"`
	Alive   scene.NodeRef            `aego:"alive"`
	Missing scene.NodeRef            `aego:"missing"`
	NoCam   scene.NodeRef            `aego:"noCam"`
	NoTex   asset.Ref[asset.Texture] `aego:"noTex,asset=texture"`
	Lost    asset.Ref[asset.Texture] `aego:"lost,asset=texture"`
}

type goldenModule struct{}

func (goldenModule) Info() module.Info {
	return module.Info{ID: "golden"}
}

func (goldenModule) Register(r *module.Registry) error {
	return module.Component[goldenRefs](r, schema.Name("golden.Refs")).Err()
}

func TestEditorInspectorRefs(t *testing.T) {
	env := newEditorEnv(t)
	types, err := builtin.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Load(types, bind.NewManifest(), stdlib.Options{}, goldenModule{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	td, _ := types.Schema().ByName("golden.Refs")
	fd, _ := td.Field("noCam")
	fd.Component = "engine.Camera2D"

	s := scene.New(types, nil, scene.Options{})
	holder := s.Create(s.Root(), "Holder")
	target := s.Create(s.Root(), "Target")
	gone := s.Create(s.Root(), "Gone")
	refs := scene.Add[goldenRefs](s, holder)
	refs.Alive = scene.NewNodeRef(s.ID(target))
	refs.Missing = scene.NewNodeRef(s.ID(gone))
	refs.NoCam = scene.NewNodeRef(s.ID(target))
	refs.Lost.SetAssetRefID(ids.AssetID{Hi: 0x1234, Lo: 0x5678})
	s.Destroy(gone)
	s.Flush()

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
	ctx.Docs.New("scenes/refs.aescene", s)
	ctx.Select(s.ID(holder))
	reg := inspector.NewRegistry()
	if err := reg.Load(types.Schema()); err != nil {
		t.Fatal(err)
	}
	p := inspector.New(reg)
	img, err := env.draw(func(full mathx.Rect) { p.Draw(ctx, full) })
	checkGolden(t, "editor_inspector_refs", img, err)
}
