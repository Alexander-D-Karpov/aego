package animation

import (
	"errors"

	"aego/asset"
	"aego/core/ids"
	"aego/engine/module"
	"aego/schema"
	"aego/script/bind"
)

const (
	NamePlayer = ID + ".AnimationPlayer"
	Category   = "Animation"
)

type ClipRef struct {
	ID ids.AssetID
}

func (r *ClipRef) AssetRefID() ids.AssetID      { return r.ID }
func (r *ClipRef) SetAssetRefID(id ids.AssetID) { r.ID = id }
func (r *ClipRef) AssetKind() string            { return string(asset.ArtAnim) }
func (r ClipRef) IsZero() bool                  { return r.ID.IsZero() }

type TimeDomain uint8

const (
	DomainGame TimeDomain = iota
	DomainUnscaled
)

type LoopOverride uint8

const (
	LoopClip LoopOverride = iota
	LoopOnce
	LoopRepeat
	LoopPingPong
)

const (
	cmdPlay uint8 = 1 << iota
	cmdStop
	cmdPause
	cmdResume
	cmdSeek
)

type playerState struct {
	time    float64
	shown   float64
	seek    float32
	slot    int32
	cmd     uint8
	playing bool
	paused  bool
	first   bool
	pfirst  bool
	started bool
	auto    bool
}

type AnimationPlayer struct {
	Clip        ClipRef      `aego:"clip"`
	PlayOnStart bool         `aego:"playOnStart"`
	Speed       float32      `aego:"speed"`
	Loop        LoopOverride `aego:"loop"`
	TimeDomain  TimeDomain   `aego:"timeDomain"`
	Time        float32      `aego:"time,transient,readonly,category=Runtime"`
	Playing     bool         `aego:"playing,transient,readonly,category=Runtime"`
	Cycle       int64        `aego:"cycle,transient,readonly,category=Runtime"`

	st playerState
}

func DefaultPlayer() AnimationPlayer {
	return AnimationPlayer{PlayOnStart: true, Speed: 1}
}

func (p *AnimationPlayer) Play(clip ClipRef) {
	if !clip.IsZero() {
		p.Clip = clip
	}
	p.st.cmd = p.st.cmd&^(cmdStop|cmdPause|cmdResume) | cmdPlay
}

func (p *AnimationPlayer) Stop() {
	p.st.cmd = p.st.cmd&^(cmdPlay|cmdPause|cmdResume|cmdSeek) | cmdStop
}

func (p *AnimationPlayer) Pause() {
	p.st.cmd = p.st.cmd&^cmdResume | cmdPause
}

func (p *AnimationPlayer) Resume() {
	p.st.cmd = p.st.cmd&^cmdPause | cmdResume
}

func (p *AnimationPlayer) Seek(t float32) {
	p.st.seek = t
	p.st.cmd |= cmdSeek
}

func registerTypes(r *module.Registry, m *Module) error {
	if err := errors.Join(
		module.Enum[TimeDomain](r, ID+".TimeDomain",
			schema.EnumValue{Name: "game", Value: int64(DomainGame)},
			schema.EnumValue{Name: "unscaled", Value: int64(DomainUnscaled)}),
		module.Enum[LoopOverride](r, ID+".LoopOverride",
			schema.EnumValue{Name: "clip", Value: int64(LoopClip)},
			schema.EnumValue{Name: "once", Value: int64(LoopOnce)},
			schema.EnumValue{Name: "loop", Value: int64(LoopRepeat)},
			schema.EnumValue{Name: "pingPong", Value: int64(LoopPingPong)}),
	); err != nil {
		return err
	}
	b := module.Component[AnimationPlayer](r, schema.Name(NamePlayer),
		schema.Doc("Plays an animation clip on its node and the nodes below it: property tracks, sprite frames and events. Clip time, gameplay events and tracks other than the transform pose advance on fixed steps, so a replay gives the same result at any frame rate."),
		schema.FieldDoc("clip", "Clip to play."),
		schema.FieldDoc("playOnStart", "Starts the clip when Play starts."),
		schema.FieldDoc("speed", "Playback speed: 1 is normal, 2 twice as fast; a negative speed plays backwards."),
		schema.FieldDoc("loop", "clip keeps the loop mode of the clip; once, loop and pingPong override it."),
		schema.FieldDoc("timeDomain", "game follows game time and pauses with it; unscaled runs in real time while paused, for menus and other presentation."),
		schema.FieldDoc("time", "Time inside the clip, in seconds."),
		schema.FieldDoc("playing", "Whether the clip plays."),
		schema.FieldDoc("cycle", "Cycles of a looping clip so far."),
		schema.Runtime("time", "playing", "cycle"),
	).Category(Category).Display("Animation Player").Icon("play.start").Default(DefaultPlayer)

	b.Proc1("Play", func(_ *bind.Ctx, p *AnimationPlayer, clip ClipRef) { p.Play(clip) },
		bind.Doc("Play starts a clip from its beginning on the next fixed step; nil plays the current clip again."), bind.Params("clip"),
		bind.Example("node.Get<AnimationPlayer>().Play(Attack)"))
	b.Proc0("Stop", func(_ *bind.Ctx, p *AnimationPlayer) { p.Stop() },
		bind.Doc("Stop ends the clip on the next fixed step: tracks with onEnd=restore get back the values they had before Play, Finished runs with cycle -1."))
	b.Proc0("Pause", func(_ *bind.Ctx, p *AnimationPlayer) { p.Pause() },
		bind.Doc("Pause holds the clip where it is."))
	b.Proc0("Resume", func(_ *bind.Ctx, p *AnimationPlayer) { p.Resume() },
		bind.Doc("Resume continues a paused clip."))
	b.Proc1("Seek", func(_ *bind.Ctx, p *AnimationPlayer, t float32) { p.Seek(t) },
		bind.Doc("Seek jumps to a time inside the clip on the next fixed step without firing the events in between."), bind.Params("time"))
	b.Proc1("SetSpeed", func(_ *bind.Ctx, p *AnimationPlayer, v float32) { p.Speed = v },
		bind.Doc("SetSpeed changes the playback speed; a negative speed plays backwards."), bind.Params("speed"))

	m.sig.finished = b.Signal1[int64]("Finished", "Finished runs when a clip that does not loop ends, with cycle 0, or after Stop, with cycle -1.", "cycle")
	m.sig.event = b.Signal2[int64, float32]("Event", "Event runs when the clip reaches an event; anim.EventName(id) gives its name.", "id", "payload")
	return b.Err()
}
