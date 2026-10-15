package vectors

import (
	"strconv"

	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/shape"
	"aego/platform/input"
)

const (
	CommandToggle = "view.vectors"
	CommandFixed  = "view.vectors.fixed"
	Prefix        = "vectors."
	Category      = "Vectors"
	owner         = "editor.vectors"
)

var scales = [...]float32{0.02, 0.05, 0.1, 0.25, 0.5, 1}

func Overlay(ctx *context.Editor) shape.Overlay {
	sink := &context.VectorSink{Editor: ctx}
	return func(c *shape.OverlayCtx) {
		o := &ctx.Overlay
		if !o.On(CommandToggle) || c.Mode3D || len(ctx.Vectors) == 0 {
			return
		}
		sink.Scene, sink.World, sink.Debug, sink.View, sink.Px = c.Scene, c.World, c.Debug, c.View, c.Px
		sink.Scale, sink.Fixed = o.VectorScale(), o.Fixed
		for i := range ctx.Vectors {
			l := &ctx.Vectors[i]
			if o.On(Prefix + l.ID) {
				l.Draw(sink)
			}
		}
	}
}

func Register(ctx *context.Editor, shapes *shape.Registry) error {
	shapes.AddOverlayIn(owner, Overlay(ctx), true)
	o := &ctx.Overlay
	list := []command.Desc{{
		ID: CommandToggle, Name: "Show Vectors", Category: Category, Order: 100,
		Scope: command.ScopeScene, Shortcut: command.Key(input.KeyV, 0),
		Checked: func() bool { return o.On(CommandToggle) },
		Run: func() error {
			o.Toggle(CommandToggle)
			return nil
		},
	}}
	for i, s := range scales {
		sc := s
		list = append(list, command.Desc{
			ID:       Prefix + "scale." + strconv.Itoa(i),
			Name:     "Scale " + strconv.FormatFloat(float64(sc), 'g', -1, 32),
			Category: Category,
			Order:    400 + i,
			Checked:  func() bool { return o.VectorScale() == sc },
			Run: func() error {
				o.Scale = sc
				return nil
			},
		})
	}
	list = append(list, command.Desc{
		ID: CommandFixed, Name: "Fixed Length", Category: Category, Order: 500,
		Checked: func() bool { return o.Fixed },
		Run: func() error {
			o.Fixed = !o.Fixed
			return nil
		},
	})
	for _, d := range list {
		if err := ctx.Commands.Register(d); err != nil {
			return err
		}
	}
	return nil
}
