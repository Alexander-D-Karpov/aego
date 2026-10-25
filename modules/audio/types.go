package audio

import (
	"errors"

	"aego/asset"
	"aego/core/curve"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/data"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/scene"
	"aego/schema"
	"aego/script/bind"
)

const (
	NameListener   = ID + ".AudioListener2D"
	NameSource     = ID + ".AudioSource2D"
	NameZone       = ID + ".AudioZone2D"
	NameEvent      = ID + ".AudioEvent"
	ClipKind       = "Clip"
	Category       = "Audio"
	CodeFormat     = "AUD001"
	CodeLoop       = "AUD002"
	CodeNoClip     = "AUD003"
	CodeNoListener = "AUD004"
	CodeListeners  = "AUD005"
	CodeEvent      = "AUD006"
	busCount       = 6
)

type Bus uint8

const (
	BusMaster Bus = iota
	BusMusic
	BusSfx
	BusUI
	BusAmbience
	BusVoice
)

type Steal uint8

const (
	StealOldest Steal = iota
	StealQuietest
	StealNone
)

type PauseWith uint8

const (
	PauseGame PauseWith = iota
	PauseNever
)

type ClipRef struct {
	ID ids.AssetID
}

func (r *ClipRef) AssetRefID() ids.AssetID      { return r.ID }
func (r *ClipRef) SetAssetRefID(id ids.AssetID) { r.ID = id }
func (r *ClipRef) AssetKind() string            { return string(asset.ArtAudio) }
func (r ClipRef) IsZero() bool                  { return r.ID.IsZero() }

type Event struct {
	Variants    []ClipRef   `aego:"variants"`
	Volume      float32     `aego:"volume,min=0,max=2"`
	VolumeRand  float32     `aego:"volumeRandom,min=0,max=1"`
	Pitch       float32     `aego:"pitch,min=0.25,max=4"`
	PitchRand   float32     `aego:"pitchRandom,min=0,max=1"`
	Bus         Bus         `aego:"bus"`
	Priority    int32       `aego:"priority,min=0,max=255"`
	MaxVoices   int32       `aego:"maxVoices,min=0,max=64"`
	Steal       Steal       `aego:"steal"`
	Cooldown    float32     `aego:"cooldown,min=0"`
	Spatial     bool        `aego:"spatial"`
	MinDistance float32     `aego:"minDistance,min=0"`
	MaxDistance float32     `aego:"maxDistance,min=0"`
	Rolloff     curve.Curve `aego:"rolloff"`
	PanWidth    float32     `aego:"panWidth,min=0,max=1"`
	Loop        bool        `aego:"loop"`
	PauseWith   PauseWith   `aego:"pauseWith"`
}

type AudioListener2D struct {
	Active bool `aego:"active"`
}

type AudioSource2D struct {
	Clip        ClipRef         `aego:"clip"`
	Event       data.Ref[Event] `aego:"event"`
	Volume      float32         `aego:"volume,min=0,max=2"`
	Pitch       float32         `aego:"pitch,min=0.25,max=4"`
	Pan         float32         `aego:"pan,min=-1,max=1"`
	Bus         Bus             `aego:"bus"`
	Loop        bool            `aego:"loop"`
	PlayOnStart bool            `aego:"playOnStart"`
	Spatial     bool            `aego:"spatial"`
	MinDistance float32         `aego:"minDistance,min=0"`
	MaxDistance float32         `aego:"maxDistance,min=0"`
	Rolloff     curve.Curve     `aego:"rolloff"`
	PanWidth    float32         `aego:"panWidth,min=0,max=1"`
	PauseWith   PauseWith       `aego:"pauseWith"`
	Playing     bool            `aego:"playing"`
	Position    float32         `aego:"position"`
}

type AudioZone2D struct {
	Area     mathx.Rect `aego:"area"`
	Bus      Bus        `aego:"bus"`
	Gain     float32    `aego:"gain,min=0,max=1"`
	Priority int32      `aego:"priority"`
	FadeTime float32    `aego:"fadeTime,min=0"`
}

func DefaultRolloff() curve.Curve {
	return curve.Keyed(curve.Key{T: 0, V: 1}, curve.Key{T: 0.5, V: 0.25}, curve.Key{T: 1, V: 0})
}

func DefaultEvent() Event {
	return Event{Volume: 1, Pitch: 1, Bus: BusSfx, MinDistance: 1, MaxDistance: 10, Rolloff: DefaultRolloff(), PanWidth: 0.8}
}

func DefaultSource() AudioSource2D {
	return AudioSource2D{Volume: 1, Pitch: 1, Bus: BusSfx, MinDistance: 1, MaxDistance: 10, Rolloff: DefaultRolloff(), PanWidth: 0.8}
}

func DefaultListener() AudioListener2D {
	return AudioListener2D{Active: true}
}

func DefaultZone() AudioZone2D {
	return AudioZone2D{Area: mathx.Rect{X: -4, Y: -3, W: 8, H: 6}, Bus: BusMusic, Gain: 0.5, FadeTime: 0.5}
}

func diag(code string, sev problems.Severity, msg string) problems.Diagnostic {
	return problems.Diagnostic{Code: code, Severity: sev, Message: msg}
}

func validateEvent(e *Event) []problems.Diagnostic {
	var out []problems.Diagnostic
	n := 0
	for _, v := range e.Variants {
		if !v.IsZero() {
			n++
		}
	}
	if n == 0 {
		out = append(out, diag(CodeEvent, problems.Error, "Audio Event has no variants to play"))
	}
	if e.Spatial && e.MaxDistance <= e.MinDistance {
		out = append(out, diag(CodeEvent, problems.Error, "Audio Event: Max Distance must be larger than Min Distance for a spatial sound"))
	}
	return out
}

func validateSource(ctx module.ValidateContext, s *AudioSource2D) []problems.Diagnostic {
	var out []problems.Diagnostic
	if s.Clip.IsZero() && s.Event.IsZero() {
		out = append(out, diag(CodeNoClip, problems.Warning, "Audio Source 2D has neither a Clip nor an Event"))
	}
	if !s.Event.IsZero() && !s.Clip.IsZero() {
		out = append(out, diag(CodeNoClip, problems.Info, "Audio Source 2D plays its Event; Clip and the playback fields are ignored"))
	}
	if s.Spatial {
		has := false
		scene.Query1[AudioListener2D](ctx.Scene).Each(func(_ scene.NodeHandle, l *AudioListener2D) {
			has = has || l.Active
		})
		if !has {
			out = append(out, diag(CodeNoListener, problems.Info, "the scene has no active Audio Listener 2D; spatial sound is heard from the camera"))
		}
	}
	return out
}

func validateListener(ctx module.ValidateContext, l *AudioListener2D) []problems.Diagnostic {
	if !l.Active {
		return nil
	}
	for _, h := range ctx.Scene.PreOrder() {
		if o, ok := scene.Get[AudioListener2D](ctx.Scene, h); ok && o.Active {
			if h == ctx.Node {
				return nil
			}
			return []problems.Diagnostic{diag(CodeListeners, problems.Warning, "another active Audio Listener 2D comes first in the hierarchy; this one is ignored")}
		}
	}
	return nil
}

func checkClip(c *module.AssetCheck) []problems.Diagnostic {
	a, err := asset.DecodeAudio(c.Source)
	if err != nil {
		return []problems.Diagnostic{diag(CodeFormat, problems.Error, err.Error())}
	}
	if a.LoopStart < 0 || (a.LoopEnd > 0 && (a.LoopEnd > a.Frames || a.LoopStart >= a.LoopEnd)) {
		return []problems.Diagnostic{diag(CodeLoop, problems.Warning, "the loop points are outside the sound; the whole sound loops instead")}
	}
	return nil
}

func enumOf(names []string) []schema.EnumValue {
	out := make([]schema.EnumValue, len(names))
	for i, n := range names {
		out[i] = schema.EnumValue{Name: n, Value: int64(i)}
	}
	return out
}

func (m *Module) registerTypes(r *module.Registry) error {
	list := []error{
		module.Enum[Bus](r, ID+".Bus", enumOf(Buses)...),
		module.Enum[Steal](r, ID+".Steal", enumOf([]string{"oldest", "quietest", "none"})...),
		module.Enum[PauseWith](r, ID+".PauseWith", enumOf([]string{"game", "never"})...),
	}
	ev := module.DataType[Event](r, schema.Name(NameEvent),
		schema.Doc("A sound to play with its variants, randomness, bus, voice limit and space."),
		schema.FieldDoc("variants", "Clips; one is picked at random on every play."),
		schema.FieldDoc("volume", "Volume of the event, before the bus."),
		schema.FieldDoc("volumeRandom", "Each play is quieter by up to this fraction."),
		schema.FieldDoc("pitch", "Playback speed and pitch; 2 is an octave up."),
		schema.FieldDoc("pitchRandom", "Each play changes the pitch by up to this fraction up or down."),
		schema.FieldDoc("bus", "Mixer bus the event plays on."),
		schema.FieldDoc("priority", "When the mixer runs out of voices, lower priorities are stolen first."),
		schema.FieldDoc("maxVoices", "Most voices of this event at once in a world; 0 is unlimited."),
		schema.FieldDoc("steal", "What happens at Max Voices: stop the oldest, stop the quietest, or skip the new play."),
		schema.FieldDoc("cooldown", "Seconds of game time before the event can play again."),
		schema.FieldDoc("spatial", "Fades and pans with the distance to the listener."),
		schema.FieldDoc("minDistance", "Distance in world units up to which the sound is at full volume."),
		schema.FieldDoc("maxDistance", "Distance in world units at which the sound is silent and fully panned."),
		schema.FieldDoc("rolloff", "Volume from Min Distance (time 0) to Max Distance (time 1)."),
		schema.FieldDoc("panWidth", "How far a sound at Max Distance to the side is panned, 0 to 1."),
		schema.FieldDoc("loop", "Loops until stopped."),
		schema.FieldDoc("pauseWith", "game pauses with the world, never keeps playing."),
	).Display("Audio Event").Category(Category).Default(DefaultEvent).Validate(validateEvent)
	space := schema.Space(schema.Space2D)
	lis := module.Component[AudioListener2D](r, schema.Name(NameListener), space,
		schema.Doc("Hears spatial sound from its node. Without one the camera of the main output listens."),
		schema.FieldDoc("active", "The first active listener in the hierarchy is used."),
	).Category(Category).Display("Audio Listener 2D").Icon("audio.listener").Default(DefaultListener).Validate(validateListener)
	src := module.Component[AudioSource2D](r, schema.Name(NameSource), space, schema.Runtime("playing", "position"),
		schema.Doc("Plays a clip or an audio event from its node."),
		schema.FieldDoc("clip", "Sound to play; ignored when Event is set."),
		schema.FieldDoc("event", "Audio event to play instead of Clip; its settings replace the fields below."),
		schema.FieldDoc("volume", "Volume before the bus."),
		schema.FieldDoc("pitch", "Playback speed and pitch."),
		schema.FieldDoc("pan", "Left -1 to right 1, for a sound that is not spatial."),
		schema.FieldDoc("bus", "Mixer bus."),
		schema.FieldDoc("loop", "Loops until stopped."),
		schema.FieldDoc("playOnStart", "Plays when Play starts."),
		schema.FieldDoc("spatial", "Fades and pans with the distance to the listener."),
		schema.FieldDoc("minDistance", "Full volume up to this distance, in world units."),
		schema.FieldDoc("maxDistance", "Silent from this distance, in world units."),
		schema.FieldDoc("rolloff", "Volume from Min Distance (time 0) to Max Distance (time 1)."),
		schema.FieldDoc("panWidth", "Pan at Max Distance to the side."),
		schema.FieldDoc("pauseWith", "game pauses with the world, never keeps playing."),
		schema.FieldDoc("playing", "True while a sound of this source plays."),
		schema.FieldDoc("position", "Seconds into the sound."),
	).Category(Category).Display("Audio Source 2D").Icon("audio.source").NodeIcon(70).Default(DefaultSource).Validate(validateSource)
	m.started = src.Signal0("Started", "Started runs when the source starts a sound.")
	m.finished = src.Signal1[bool]("Finished", "Finished runs when the sound of the source ends; finished is false after Stop or when it was cut off.", "finished")
	src.Proc0("Play", func(c *bind.Ctx, _ *AudioSource2D) {
		if s := need(c); s != nil {
			s.PlaySource(c.Recv)
		}
	}, bind.Doc("Play starts the source from the beginning, stopping a sound it already plays."))
	src.Proc0("Stop", func(c *bind.Ctx, _ *AudioSource2D) {
		if s := live(c); s != nil {
			s.StopSource(c.Recv, 0)
		}
	}, bind.Doc("Stop ends the sound of the source with a short fade."))
	src.Proc1("StopFade", func(c *bind.Ctx, _ *AudioSource2D, sec float32) {
		if s := live(c); s != nil {
			s.StopSource(c.Recv, sec)
		}
	}, bind.Doc("StopFade fades the sound of the source out over seconds."), bind.Params("seconds"))
	src.Method0("IsPlaying", func(c *bind.Ctx, _ *AudioSource2D) bool {
		s := live(c)
		return s != nil && s.SourcePlaying(c.Recv)
	}, bind.Doc("IsPlaying reports whether the source plays a sound."))
	zone := module.Component[AudioZone2D](r, schema.Name(NameZone), space,
		schema.Doc("While the listener is inside Area, Bus is turned to Gain."),
		schema.FieldDoc("area", "Rectangle in this node's space."),
		schema.FieldDoc("bus", "Bus the zone turns down or up."),
		schema.FieldDoc("gain", "Volume of the bus inside the zone, 0 to 1."),
		schema.FieldDoc("priority", "Among overlapping zones on one bus the highest priority wins, then the one entered later."),
		schema.FieldDoc("fadeTime", "Seconds to fade in and out of the zone."),
	).Category(Category).Display("Audio Zone 2D").Icon("audio.zone").Default(DefaultZone)
	list = append(list, ev.Err(), lis.Err(), src.Err(), zone.Err())
	return errors.Join(list...)
}
