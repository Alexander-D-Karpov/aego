package editor

import (
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/modules/gameplay"
	"aego/scene"
	"aego/ui/widgets"
)

func one[T any](ctx *context.Editor, nodes []ids.NodeID) (*T, bool) {
	d := ctx.Doc()
	if d == nil || len(nodes) != 1 {
		return nil, false
	}
	return scene.Get[T](d.Scene(), d.Scene().Handle(nodes[0]))
}

func flag(on bool) string {
	if on {
		return "●"
	}
	return "○"
}

func (p *plugin) motionInspector(ctx *context.Editor, nodes []ids.NodeID, def func()) {
	def()
	m, ok := one[gameplay.Motion2D](ctx, nodes)
	if !ok || (m.Velocity == (mathx.Vec2{}) && !m.Grounded && !m.OnWall && !m.OnCeiling) {
		return
	}
	widgets.Labelf(ctx.UI, "Grounded %s   Wall %s   Ceiling %s", flag(m.Grounded), flag(m.OnWall), flag(m.OnCeiling))
	widgets.Labelf(ctx.UI, "Speed %.2f   Floor %.0f°", m.Velocity.Len(), m.FloorAngle)
}

func (p *plugin) healthInspector(ctx *context.Editor, nodes []ids.NodeID, def func()) {
	def()
	hp, ok := one[gameplay.Health](ctx, nodes)
	if !ok || hp.Max <= 0 {
		return
	}
	ui := ctx.UI
	th := &ui.Theme
	cur := hp.Current
	if cur == 0 && !hp.Dead {
		cur = hp.Max
	}
	bar := ui.Cursor().Next(6)
	ui.RoundRect(bar, 3, th.Widget)
	k := mathx.Clamp(float32(cur)/float32(hp.Max), 0, 1)
	ui.RoundRect(mathx.Rect{X: bar.X, Y: bar.Y, W: bar.W * k, H: bar.H}, 3, colHealth)
	widgets.Labelf(ui, "%d / %d", cur, hp.Max)
}

func (p *plugin) fsmInspector(ctx *context.Editor, nodes []ids.NodeID, def func()) {
	def()
	m, ok := one[gameplay.StateMachine](ctx, nodes)
	if !ok || m.Current == "" {
		return
	}
	widgets.Labelf(ctx.UI, "State %s for %.1f s, before %s", m.Current, m.Time, m.Previous)
}
