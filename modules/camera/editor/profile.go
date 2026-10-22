package editor

import (
	"strconv"

	"aego/editor/core/context"
	"aego/editor/core/props"
	"aego/modules/camera"
	"aego/ui/widgets"
)

func (p *plugin) profile(ed *context.Editor) {
	ui := ed.UI
	c := camera.Of(ed.Docs.ActiveWorld())
	s := ed.Docs.ActiveScene()
	if c == nil || s == nil {
		widgets.LabelDim(ui, "Cameras run only in Play")
		return
	}
	n := 0
	c.Outputs(func(o camera.OutputInfo) {
		n++
		name := o.Name
		if name == "" {
			name = "main"
		}
		_, vc, _ := props.NodeLabel(s, o.Active, false)
		if vc == "" {
			vc = "no camera"
		}
		line := name + ": " + vc + ", zoom " + strconv.FormatFloat(float64(o.Logical.Zoom), 'f', 2, 32)
		if o.Blend > 0 {
			line += ", blending " + strconv.Itoa(int(o.Blend*100)) + "%"
		}
		widgets.Label(ui, line)
	})
	if n == 0 {
		widgets.LabelDim(ui, "No camera outputs")
	}
}
