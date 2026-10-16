package editor

import (
	"fmt"

	"aego/core/ids"
	"aego/editor/core/context"
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
		widgets.LabelDim(ui, "Physics is not attached to the play world")
		return
	}
	st := sim.Stats()
	widgets.LabelDim(ui, fmt.Sprintf("bodies %d   active %d   sleeping %d   shapes %d   joints %d", st.Bodies, st.Active, st.Sleeping, st.Shapes, st.Joints))
	widgets.LabelDim(ui, fmt.Sprintf("candidates %d   touching %d   manifolds %d   points %d   islands %d   events %d",
		st.BroadphaseCandidates, st.TouchingPairs, st.Manifolds, st.ContactPoints, st.Islands, len(sim.World().Events().Contacts)))
	t := st.Time
	widgets.LabelDim(ui, "broad "+ms(t.Broad)+"   narrow "+ms(t.Narrow)+"   solve "+ms(t.Solve)+"   integrate "+ms(t.Integrate)+"   sync "+ms(t.Sync)+"   events "+ms(t.Events)+" ms")
	widgets.LabelDim(ui, fmt.Sprintf("steps %d   touching pairs tracked %d", st.Steps, len(sim.Touches())))
}
