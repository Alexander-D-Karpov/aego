package editor

import (
	"strconv"

	"aego/editor/core/context"
	"aego/editor/core/props"
	"aego/engine/i18n"
	"aego/modules/camera"
	"aego/ui/widgets"
)

func (p *plugin) profile(ed *context.Editor) {
	ui := ed.UI
	c := camera.Of(ed.Docs.ActiveWorld())
	s := ed.Docs.ActiveScene()
	if c == nil || s == nil {
		widgets.LabelDim(ui, i18n.Tr("i18n.camera.cameras_run_only_in_play", "Cameras run only in Play"))
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
		line := i18n.Tr("i18n.camera.name_vc_zoom_formatfloat", "{name}: {vc}, zoom {formatfloat}", i18n.Str("name", name), i18n.Str("vc", vc), i18n.Str("formatfloat", strconv.FormatFloat(float64(o.Logical.Zoom), 'f', 2, 32)))
		if o.Blend > 0 {
			line += i18n.Tr("i18n.camera.blending_v", ", blending {v}%", i18n.Str("v", strconv.Itoa(int(o.Blend*100))))
		}
		widgets.Label(ui, line)
	})
	if n == 0 {
		widgets.LabelDim(ui, i18n.Tr("i18n.camera.no_camera_outputs", "No camera outputs"))
	}
}
