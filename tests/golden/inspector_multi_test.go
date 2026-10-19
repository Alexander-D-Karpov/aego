//go:build aego_golden

package golden

import (
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/clipboard"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/selection"
	"aego/editor/core/services"
	"aego/editor/ext/inspect"
	"aego/editor/panels/inspector"
	"aego/scene"
	"aego/script/bind"
	"aego/ui/dnd"
)

func TestEditorInspectorMultiLocked(t *testing.T) {
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
	s := scene.New(types, nil, scene.Options{})
	a := s.Create(s.Root(), "Slime")
	b := s.Create(s.Root(), "Bat")
	sa := scene.Add[goldenStats](s, a)
	sa.Speed, sa.Tint = 2, mathx.Color{R: 0.3, G: 0.8, B: 0.4, A: 1}
	sb := scene.Add[goldenStats](s, b)
	sb.Speed, sb.Tint = 5, sa.Tint
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
	ctx.Docs.New("scenes/multi.aescene", s)
	ctx.Select(s.ID(a), s.ID(b))
	reg := inspect.NewRegistry()
	if err := reg.Load(types.Schema()); err != nil {
		t.Fatal(err)
	}
	reg.SetEditor(ctx)
	p := inspector.New(reg, inspector.Deps{
		Docs:      ctx,
		Selection: ctx,
		Commands:  ctx.Commands,
		Notices:   ctx,
		Nav:       ctx,
		Problems:  ctx,
		Clip:      clipboard.EnvOf(ctx),
		Collapsed: ctx.Collapsed,
	})
	p.ToggleLock()
	ctx.Selection.Clear()
	img, err := env.draw(func(full mathx.Rect) { p.Draw(&services.Frame{UI: env.ui, DnD: ctx.DnD}, full) })
	checkGolden(t, "editor_inspector_multi_locked", img, err)
}
