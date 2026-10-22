//go:build aego_golden

package golden

import (
	"math"
	"testing"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/format/aetext"
	audioed "aego/modules/audio/editor"
)

func TestEditorInspectorClip(t *testing.T) {
	env := newEditorEnv(t)
	ctx := &context.Editor{UI: env.ui, Settings: context.DefaultSettings(), Collapsed: map[string]bool{}}
	peaks := make([]float32, 256)
	for i := range peaks {
		f := float64(i)
		peaks[i] = float32(0.08 + 0.85*math.Abs(math.Sin(f*0.11))*math.Exp(-f/200))
	}
	v := audioed.ClipView{
		ID: ids.AssetID{Hi: 0xa1, Lo: 1}, Path: "assets/sounds/jump.wav", Format: asset.AudioWAV,
		Rate: 48000, Channels: 1, Frames: 48000, Seconds: 1, Peaks: peaks, PeaksDone: true,
		Settings: pipeline.Settings{
			"stream": aetext.Ident("auto"), "gain": aetext.Float64(0.8), "bus": aetext.Ident("sfx"),
			"loopStart": aetext.Int(12000), "loopEnd": aetext.Int(36000),
		},
		Position: 0.4, Playing: true,
	}
	img, err := env.draw(func(mathx.Rect) { audioed.DrawClip(ctx, v) })
	checkGolden(t, "editor_inspector_clip", img, err)
}
