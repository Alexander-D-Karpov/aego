package editor

import (
	"aego/editor/core/context"
	"aego/engine/i18n"
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
		widgets.LabelDim(ui, i18n.Tr("i18n.gameplay.gameplay_runs_only_in_play", "Gameplay runs only in Play"))
		return
	}
	st := s.Stats()
	prev := p.prof.last
	steps := st.Steps - prev.Steps
	widgets.Label(ui, i18n.Tr("i18n.gameplay.profile_controllers", "Controllers {controllers} per step, casts {casts} per step", i18n.Float("controllers", float64(perStep(st.Controllers, prev.Controllers, steps)), 0), i18n.Float("casts", float64(perStep(st.Casts, prev.Casts, steps)), 1)))
	widgets.Label(ui, i18n.Tr("i18n.gameplay.profile_events", "Hits {hits}, deaths {deaths}, respawns {respawns}, pickups {pickups}", i18n.Int("hits", int64(st.Hits)), i18n.Int("deaths", int64(st.Deaths)), i18n.Int("respawns", int64(st.Respawns)), i18n.Int("pickups", int64(st.Pickups))))
	p.prof.last = st
}
