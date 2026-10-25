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
	widgets.LabelDim(ui, i18n.Tr("i18n.physics.stats_bodies", "bodies {bodies}   active {active}   sleeping {sleeping}   shapes {shapes}   joints {joints}", i18n.Int("bodies", int64(st.Bodies)), i18n.Int("active", int64(st.Active)), i18n.Int("sleeping", int64(st.Sleeping)), i18n.Int("shapes", int64(st.Shapes)), i18n.Int("joints", int64(st.Joints))))
	widgets.LabelDim(ui, i18n.Tr("i18n.physics.stats_contacts", "candidates {candidates}   touching {touching}   manifolds {manifolds}   points {points}   islands {islands}   events {events}", i18n.Int("candidates", int64(st.BroadphaseCandidates)), i18n.Int("touching", int64(st.TouchingPairs)), i18n.Int("manifolds", int64(st.Manifolds)), i18n.Int("points", int64(st.ContactPoints)), i18n.Int("islands", int64(st.Islands)), i18n.Int("events", int64(len(sim.World().Events().Contacts)))))
	t := st.Time
	widgets.LabelDim(ui, i18n.Tr("i18n.physics.broad_broad_narrow_narrow_solve", "broad {broad}   narrow {narrow}   solve {solve}   integrate {integrate}   sync {sync}   events {events} ms", i18n.Str("broad", ms(t.Broad)), i18n.Str("narrow", ms(t.Narrow)), i18n.Str("solve", ms(t.Solve)), i18n.Str("integrate", ms(t.Integrate)), i18n.Str("sync", ms(t.Sync)), i18n.Str("events", ms(t.Events))))
	widgets.LabelDim(ui, i18n.Tr("i18n.physics.stats_steps", "steps {steps}   touching pairs tracked {pairs}", i18n.Int("steps", int64(st.Steps)), i18n.Int("pairs", int64(len(sim.Touches())))))
}
