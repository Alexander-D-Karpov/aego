package editor

import (
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/services"
	"aego/editor/core/shape"
	"aego/editor/core/workspace"
	"aego/editor/ext"
	"aego/engine/i18n"
	"aego/modules/debug"
	"aego/ui/core"
	"aego/ui/widgets"
)

const PanelID = debug.ID + ".panel"

var presets = [...]struct {
	name  i18n.Text
	match func(string) bool
}{
	{i18n.T("i18n.debug.preset.all", "All"), func(string) bool { return true }},
	{i18n.T("i18n.debug.preset.none", "None"), func(string) bool { return false }},
	{i18n.T("i18n.debug.preset.collision", "Collision"), func(n string) bool { return strings.HasPrefix(n, "physics.") }},
	{i18n.T("i18n.debug.preset.velocity", "Velocity"), func(n string) bool { return strings.Contains(n, "velocity") }},
	{i18n.Text{En: "AI"}, func(n string) bool { return strings.HasPrefix(n, "ai") }},
	{i18n.T("i18n.debug.preset.camera", "Camera"), func(n string) bool { return strings.HasPrefix(n, "camera") }},
}

type plugin struct {
	ed     *context.Editor
	hidden map[string]bool
}

func Hook(e *ext.Registry) error {
	p := &plugin{ed: e.Editor(), hidden: map[string]bool{}}
	e.Overlay(p.overlay)
	return e.Panel(workspace.Desc{Panel: p, Near: workspace.Console, Icon: "panel.debug_draw"})
}

func (p *plugin) svc() *debug.Service {
	if p.ed == nil {
		return nil
	}
	return debug.Of(p.ed.PlayWorld())
}

func (p *plugin) apply(s *debug.Service) {
	for ch, h := range p.hidden {
		s.Hide(ch, h)
	}
}

func (p *plugin) overlay(c *shape.OverlayCtx) {
	if c.Mode3D || c.World == nil {
		return
	}
	s := debug.Of(c.World)
	if s == nil {
		return
	}
	p.apply(s)
	s.Lines(c.Debug, c.Px, 1, false)
}

func (*plugin) ID() string {
	return PanelID
}

func (*plugin) Title() string {
	return i18n.Tr("i18n.debug.draw.title", "Debug Draw")
}

func (p *plugin) set(s *debug.Service, name string, on bool) {
	p.hidden[name] = !on
	s.Hide(name, !on)
}

func (p *plugin) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	s := p.svc()
	if s == nil {
		widgets.LabelDim(ui, i18n.Tr("i18n.debug.debug_channels_appear_here_while", "Debug channels appear here while Play runs"))
		return
	}
	p.apply(s)
	widgets.LabelDim(ui, i18n.Tr("i18n.debug.live_primitives_formatuint_dropped", "{live} primitives   {formatuint} dropped", i18n.Str("live", strconv.Itoa(s.Live())), i18n.Str("formatuint", strconv.FormatUint(s.Dropped(), 10))))
	row := ui.Cursor().Next(0)
	bw := row.W / float32(len(presets))
	for i, pr := range presets {
		rr := mathx.Rect{X: row.X + float32(i)*bw, Y: row.Y, W: bw - 4, H: row.H}
		if widgets.ButtonRect(ui, core.IDSeq(ui.ID("preset"), i), rr, pr.name.String()) {
			s.EachChannel(func(name string, _ int, _ uint64) { p.set(s, name, pr.match(name)) })
		}
	}
	base := ui.ID("channel")
	i := 0
	s.EachChannel(func(name string, count int, dropped uint64) {
		on := s.Wants(name)
		was := on
		label := name + "   " + strconv.Itoa(count)
		if dropped > 0 {
			label += i18n.Tr("i18n.debug.dropped_formatuint", "   dropped {formatuint}", i18n.Str("formatuint", strconv.FormatUint(dropped, 10)))
		}
		widgets.ToggleButton(ui, core.IDSeq(base, i), ui.Cursor().Next(0), label, &on)
		if on != was {
			p.set(s, name, on)
		}
		i++
	})
}
