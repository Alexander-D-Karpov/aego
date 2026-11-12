package editor

import (
	"errors"
	"math/rand/v2"

	"aego/asset"
	"aego/core/ids"
	"aego/data"
	"aego/editor/core/context"
	"aego/modules/audio"
)

var (
	errNoOutput   = errors.New("the audio output is not open")
	errNoVariants = errors.New("the audio event has no clips")
)

// audition remembers which event started the preview clip.
type audition struct {
	event ids.AssetID
	clip  ids.AssetID
}

func previewModule(ed *context.Editor) (*audio.Module, error) {
	if ed.Engine == nil || ed.Loader == nil {
		return nil, errNoOutput
	}
	if mod := audio.ModuleOf(ed.Engine); mod != nil {
		return mod, nil
	}
	return nil, errNoOutput
}

func (p *plugin) playClip(ed *context.Editor, id ids.AssetID) error {
	mod, err := previewModule(ed)
	if err != nil {
		return err
	}
	err = mod.Preview(ed.Loader, id, false)
	if err == nil || mod.PreviewClip().IsZero() {
		p.aud = audition{}
	}
	return err
}

func (p *plugin) playEvent(ed *context.Editor, id ids.AssetID) error {
	mod, err := previewModule(ed)
	if err != nil {
		return err
	}
	if ed.Types == nil {
		return errNoOutput
	}
	var src asset.DataSource = ed.Loader
	ev, err := data.Get[audio.Event](data.NewLibrary(ed.Types.Schema(), src), id)
	if err != nil {
		return err
	}
	var clips []ids.AssetID
	for _, v := range ev.Variants {
		if !v.ID.IsZero() {
			clips = append(clips, v.ID)
		}
	}
	if len(clips) == 0 {
		return errNoVariants
	}
	clip := clips[rand.IntN(len(clips))]
	if err := mod.Preview(ed.Loader, clip, ev.Loop); err != nil {
		return err
	}
	p.aud = audition{event: id, clip: clip}
	return nil
}

func (p *plugin) stopAudition(ed *context.Editor) {
	if mod, err := previewModule(ed); err == nil {
		if err := mod.StopPreview(); err != nil {
			p.notify(err)
			return
		}
	}
	p.aud = audition{}
}

func playingClip(ed *context.Editor) ids.AssetID {
	mod, err := previewModule(ed)
	if err != nil {
		return ids.AssetID{}
	}
	if _, live := mod.PreviewPosition(); !live {
		return ids.AssetID{}
	}
	return mod.PreviewClip()
}

func (p *plugin) clipPlaying(ed *context.Editor) ids.AssetID {
	if !p.aud.event.IsZero() {
		return ids.AssetID{}
	}
	return playingClip(ed)
}

func (p *plugin) eventPlaying(ed *context.Editor) ids.AssetID {
	if c := playingClip(ed); !c.IsZero() && c == p.aud.clip {
		return p.aud.event
	}
	return ids.AssetID{}
}
