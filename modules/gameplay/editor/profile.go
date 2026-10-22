package editor

import (
	"aego/editor/core/context"
	"aego/modules/gameplay"
	"aego/ui/widgets"
)

type profileState struct {
	last gameplay.Stats
}

func perStep(now, prev, steps uint64) float64 {
	if steps == 0 {
		return 0
	}
	return float64(now-prev) / float64(steps)
}

func (p *plugin) profile(ed *context.Editor) {
	ui := ed.UI
	s := gameplay.Of(ed.Docs.ActiveWorld())
	if s == nil {
		widgets.LabelDim(ui, "Gameplay runs only in Play")
		return
	}
	st := s.Stats()
	prev := p.prof.last
	steps := st.Steps - prev.Steps
	widgets.Labelf(ui, "Controllers %.0f per step, casts %.1f per step", perStep(st.Controllers, prev.Controllers, steps), perStep(st.Casts, prev.Casts, steps))
	widgets.Labelf(ui, "Hits %d, deaths %d, respawns %d, pickups %d", st.Hits, st.Deaths, st.Respawns, st.Pickups)
	p.prof.last = st
}
