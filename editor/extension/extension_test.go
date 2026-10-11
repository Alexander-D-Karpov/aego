package extension

import (
	"errors"
	"testing"

	"aego/core/mathx"
	"aego/editor/command"
	"aego/editor/context"
	"aego/editor/gizmo"
	"aego/editor/inspector"
	"aego/editor/shape"
	"aego/editor/templates"
	"aego/editor/workspace"
	"aego/scene"
	"aego/schema"
)

type boomPanel struct{}

func (boomPanel) ID() string                       { return "quest" }
func (boomPanel) Title() string                    { return "Quest" }
func (boomPanel) Draw(*context.Editor, mathx.Rect) { panic("boom") }

func hook(fail bool) func(*Registry) error {
	return func(r *Registry) error {
		if err := r.Panel(boomPanel{}); err != nil {
			return err
		}
		if err := r.Command(command.Desc{ID: "game.spawn", Run: func() error { panic("cmd") }}); err != nil {
			return err
		}
		if fail {
			return errors.New("nope")
		}
		return nil
	}
}

func setup(t *testing.T) (*Registry, Deps) {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	d := Deps{
		Editor:     &context.Editor{},
		Workspace:  workspace.New(t.TempDir(), nil),
		Inspectors: inspector.NewRegistry(),
		Gizmos:     gizmo.NewRegistry(),
		Templates:  templates.New(),
		Shapes:     shape.NewRegistry(),
		Commands:   command.New(),
		Schema:     types.Schema(),
	}
	return New(d), d
}

func TestPanickingPanelDisablesExtension(t *testing.T) {
	r, d := setup(t)
	if err := r.Run("game", hook(false)); err != nil {
		t.Fatal(err)
	}
	p, ok := d.Workspace.Panel("quest")
	if !ok {
		t.Fatal("panel not registered")
	}
	func() {
		defer func() { recover() }()
		p.Draw(d.Editor, mathx.Rect{})
	}()
	r.Flush()
	if _, ok := r.Disabled()["game"]; !ok {
		t.Fatal("extension not disabled")
	}
	if d.Workspace.Has("quest") {
		t.Fatal("panel of a disabled extension must be unregistered")
	}
	if _, ok := d.Commands.Get("game.spawn"); ok {
		t.Fatal("command of a disabled extension must be unregistered")
	}
}

func TestPanickingCommandReturnsError(t *testing.T) {
	r, d := setup(t)
	if err := r.Run("game", hook(false)); err != nil {
		t.Fatal(err)
	}
	if err := d.Commands.Run("game.spawn"); err == nil {
		t.Fatal("panic must surface as an error")
	}
	r.Flush()
	if len(r.Disabled()) != 1 {
		t.Fatal("extension not disabled")
	}
}

func TestFailedRegistrationRollsBack(t *testing.T) {
	r, d := setup(t)
	if err := r.Run("game", hook(true)); err == nil {
		t.Fatal("registration error swallowed")
	}
	if d.Workspace.Has("quest") {
		t.Fatal("panel of a failed extension must be unregistered")
	}
	if _, ok := d.Commands.Get("game.spawn"); ok {
		t.Fatal("command of a failed extension must be unregistered")
	}
	if _, ok := r.Disabled()["game"]; !ok {
		t.Fatal("failed extension must be marked disabled")
	}
}
