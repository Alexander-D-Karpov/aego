package editor

import (
	"errors"
	"math/bits"

	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/modules/audio"
	"aego/ui/core"
	"aego/ui/widgets"
)

// audioResampledFrame maps a source-frame coordinate to the nearest decoded
// memory frame without overflowing while multiplying large frame counts.
func audioResampledFrame(frame int64, sourceRate, memoryRate int) (int64, bool) {
	if frame < 0 || frame > 1<<53 || sourceRate <= 0 || memoryRate <= 0 {
		return 0, false
	}
	den := uint64(sourceRate)
	hi, lo := bits.Mul64(uint64(frame), uint64(memoryRate))
	if hi >= den {
		return 0, false
	}
	whole, rem := bits.Div64(hi, lo, den)
	if whole > 1<<53 {
		return 0, false
	}
	if rem >= den/2+den%2 {
		if whole == 1<<53 {
			return 0, false
		}
		whole++
	}
	return int64(whole), true
}

func localizeAudioSeekError(err error) error {
	if errors.Is(err, audio.ErrPreviewSeekSourceChanged) {
		return errors.New(i18n.Tr("i18n.audio.seek_stale", "The preview source is different or cannot be verified."))
	}
	return err
}

func (p *plugin) seekAudioPreview(mod *audio.Module, v ClipView, frame int64) error {
	if mod == nil || !v.Playing || v.source == nil {
		return errors.New(i18n.Tr("i18n.audio.seek_stale", "The preview source is different or cannot be verified."))
	}
	if frame < 0 || frame >= v.Frames || v.Rate <= 0 {
		return errors.New(i18n.Tr("i18n.audio.seek_range", "The requested preview position is outside the playable range."))
	}
	if frames, rate, ok := mod.PreviewMemorySeekRange(); ok {
		mapped, valid := audioResampledFrame(frame, v.Rate, rate)
		if !valid || mapped >= frames {
			return errors.New(i18n.Tr("i18n.audio.seek_range", "The requested preview position is outside the playable range."))
		}
		return localizeAudioSeekError(mod.RequestPreviewMemorySeek(v.source, mapped))
	}
	return localizeAudioSeekError(mod.RequestPreviewStreamSeek(v.source, frame))
}

// The ruler is independent of waveform selection and loop handles. A click
// affects only an already-playing preview; it does not modify the document.
func (p *plugin) seekRuler(ui *core.Context, mod *audio.Module, v ClipView, r mathx.Rect) {
	if !v.Playing || mod == nil || v.Frames <= 0 {
		return
	}
	id := ui.ID("wave-seek")
	if ui.Clicked(id, r) {
		start, end := audioWaveBounds(v.WaveStart, v.WaveEnd, v.Frames)
		if frame, ok := audioWaveFrame(ui.Mouse().X, r, start, end); ok {
			p.notify(p.seekAudioPreview(mod, v, min(frame, v.Frames-1)))
		}
	}
	widgets.Tooltip(ui, id, i18n.Tr("i18n.audio.seek_tooltip", "Click to seek the playing preview"))
}
