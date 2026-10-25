package editor

import (
	"fmt"

	"aego/core/ids"
	"aego/editor/core/context"
	"aego/engine/i18n"
	"aego/modules/physics"
	"aego/ui/widgets"
)

func (p *plugin) issue(node ids.NodeID, comp string) (string, bool) {
	if p.ed == nil || p.ed.Problems == nil {
		return "", false
	}
	list, rev := p.ed.Problems.Collect()
	if rev != p.rev {
		p.rev = rev
		clear(p.index)
		for i := range list {
			d := &list[i]
			if d.Owner == physics.ID {
				p.index[issueKey{node: d.Subject.Node, comp: d.Subject.Field}] = d.Message
			}
		}
	}
	msg, ok := p.index[issueKey{node: node, comp: comp}]
	return msg, ok
}

func ms(d interface{ Seconds() float64 }) string {
	return fmt.Sprintf("%.2f", d.Seconds()*1000)
}

func (p *plugin) profile(ctx *context.Editor) {
	ui := ctx.UI
	sim := p.playSim()
	if sim == nil {
		widgets.LabelDim(ui, i18n.Tr("i18n.physics.physics_is_not_attached_to", "Physics is not attached to the play world"))
		return
	}
	st := sim.Stats()
	widgets.LabelDim(ui, fmt.Sprintf("bodies %d   active %d   sleeping %d   shapes %d   joints %d", st.Bodies, st.Active, st.Sleeping, st.Shapes, st.Joints))
	widgets.LabelDim(ui, fmt.Sprintf("candidates %d   touching %d   manifolds %d   points %d   islands %d   events %d",
		st.BroadphaseCandidates, st.TouchingPairs, st.Manifolds, st.ContactPoints, st.Islands, len(sim.World().Events().Contacts)))
	t := st.Time
	widgets.LabelDim(ui, i18n.Tr("i18n.physics.broad_broad_narrow_narrow_solve", "broad {broad}   narrow {narrow}   solve {solve}   integrate {integrate}   sync {sync}   events {events} ms", i18n.Str("broad", ms(t.Broad)), i18n.Str("narrow", ms(t.Narrow)), i18n.Str("solve", ms(t.Solve)), i18n.Str("integrate", ms(t.Integrate)), i18n.Str("sync", ms(t.Sync)), i18n.Str("events", ms(t.Events))))
	widgets.LabelDim(ui, fmt.Sprintf("steps %d   touching pairs tracked %d", st.Steps, len(sim.Touches())))
}
