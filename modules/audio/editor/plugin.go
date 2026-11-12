package editor

import (
	"aego/asset"
	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/editor/ext"
	"aego/editor/ext/inspect"
	"aego/engine"
	"aego/engine/i18n"
	"aego/modules/audio"
	"aego/schema"
)

const (
	CmdRanges  = audio.ID + ".ranges"
	partMin    = 40
	partMax    = 41
	partZone   = 48
	mixerOrder = 178
)

var (
	colMin   = mathx.Color{R: 0.45, G: 0.85, B: 1, A: 0.9}
	colMax   = mathx.Color{R: 0.45, G: 0.85, B: 1, A: 0.45}
	colZone  = mathx.Color{R: 0.65, G: 0.55, B: 1, A: 0.85}
	colDim   = mathx.Color{R: 0.7, G: 0.7, B: 0.7, A: 0.3}
	colVoice = mathx.Color{R: 1, G: 0.75, B: 0.3, A: 0.9}
	colEar   = mathx.Color{R: 0.4, G: 1, B: 0.6, A: 0.9}
	colLow   = mathx.Color{R: 0.35, G: 0.8, B: 0.45, A: 1}
	colMid   = mathx.Color{R: 0.95, G: 0.8, B: 0.3, A: 1}
	colHigh  = mathx.Color{R: 0.95, G: 0.4, B: 0.35, A: 1}
)

type plugin struct {
	ed            *context.Editor
	mod           *audio.Module
	clip          clipEdit
	aud           audition
	auditionOwner *plugin // document views share the registered player's audition state
	workingAudio  workingAudioQueue
}

func lookup(sr *schema.Registry, name string) (*schema.TypeDescriptor, error) {
	td, ok := sr.ByName(name)
	if !ok {
		return nil, errs.New(errs.SchemaUnknownType, "audio component is not registered", log.F("type", name))
	}
	return td, nil
}

func Hook(e *ext.Registry) error {
	if err := registerEventThumbnails(e); err != nil {
		return err
	}
	p := &plugin{ed: e.Editor(), clip: clipEdit{drag: -1}}
	sr := e.Schema()
	src, err := lookup(sr, audio.NameSource)
	if err != nil {
		return err
	}
	zone, err := lookup(sr, audio.NameZone)
	if err != nil {
		return err
	}
	if err := e.Gizmo(audio.NameSource, &rangeGizmo{td: src}); err != nil {
		return err
	}
	if err := e.Gizmo(audio.NameZone, &zoneGizmo{td: zone, active: -1}); err != nil {
		return err
	}
	if err := e.Inspector(audio.NameSource, inspect.CustomFunc(p.sourceInspector)); err != nil {
		return err
	}
	if err := e.AssetInspector(string(asset.ArtAudio), p.clipInspector); err != nil {
		return err
	}
	if p.ed != nil && p.ed.Docs != nil {
		docs := p.ed.Docs
		docs.Subscribe(func(ev document.Event) {
			if ev.Kind == document.EvClosed {
				p.closeWorkingAudio(docs, ev.Doc)
			}
		})
		openClip := e.DocumentView(func(d *document.AssetDocument) func(*services.Frame, mathx.Rect) {
			// Per-tab interaction state prevents a different Inspector selection
			// from resetting a dragged loop handle or an in-progress gain edit.
			view := &plugin{ed: p.ed, clip: clipEdit{id: d.Asset, drag: -1}, auditionOwner: p}
			return func(_ *services.Frame, _ mathx.Rect) { view.clipInspectorView(p.ed, d.Selected, true) }
		})
		if err := e.DocumentHandler(document.Handler{ID: "audio.clip", Name: i18n.T("editor.document.audio", "Audio Clip"), Icon: "file.audio", Kinds: []asset.ArtifactKind{asset.ArtAudio}, Extensions: []string{".wav", ".ogg", ".mp3"}, Capabilities: document.View | document.EditMetadata | document.Preview | document.RevealSource, Open: openClip}); err != nil {
			return err
		}
	}
	if err := e.AssetPlayer(string(asset.ArtAudio), context.AssetPlayer{Play: p.playClip, Stop: p.stopAudition, Playing: p.clipPlaying}); err != nil {
		return err
	}
	if err := e.AssetPlayer(audio.NameEvent, context.AssetPlayer{Play: p.playEvent, Stop: p.stopAudition, Playing: p.eventPlaying}); err != nil {
		return err
	}
	e.Overlay(p.sceneOverlay)
	e.GameOverlay(p.gameOverlay)
	if err := e.Panel(workspace.Desc{Panel: newMixer(p), Near: workspace.Console, Order: mixerOrder, Icon: "panel.mixer"}); err != nil {
		return err
	}
	e.Profiler(i18n.T("i18n.audio.profiler", "Audio"), true, p.profile)
	if p.ed == nil {
		return nil
	}
	o := &p.ed.Overlay
	o.Set(CmdRanges, true)
	return e.Command(command.Desc{
		ID: CmdRanges, Name: "Audio Ranges", Label: i18n.T("i18n.audio.cmd.ranges", "Audio Ranges").String, Category: audio.Category, Order: 100,
		Checked: func() bool { return o.On(CmdRanges) },
		Run: func() error {
			o.Toggle(CmdRanges)
			return nil
		},
	})
}

func (p *plugin) world() *engine.World {
	if p.ed == nil || p.ed.Docs == nil {
		return nil
	}
	return p.ed.Docs.ActiveWorld()
}

func (p *plugin) module() *audio.Module {
	if p.mod == nil {
		p.mod, _ = engine.ServiceOf[*audio.Module](p.world(), audio.MixerID)
	}
	return p.mod
}

func (p *plugin) notify(err error) {
	if p.ed != nil && err != nil {
		p.ed.Notify(context.LevelError, err.Error())
	}
}

func (p *plugin) info(msg string) {
	if p.ed != nil {
		p.ed.Notify(context.LevelInfo, msg)
	}
}

func (p *plugin) ranges() bool {
	return p.ed != nil && p.ed.Overlay.On(CmdRanges)
}
