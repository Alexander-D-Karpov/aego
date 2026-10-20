package audio

import (
	"slices"

	"aego/engine"
	"aego/engine/module"
	"aego/engine/settings"
	"aego/schema"
)

const (
	ID      = "audio"
	MixerID = ID + ".mixer"
)

var (
	Buses  = []string{"master", "music", "sfx", "ui", "ambience", "voice"}
	titles = []string{"Master", "Music", "Effects", "Interface", "Ambience", "Voice"}
)

func VolumeKey(bus string) string {
	return ID + ".bus." + bus + ".volume"
}

func MuteKey(bus string) string {
	return ID + ".bus." + bus + ".mute"
}

type Module struct {
	st   *settings.Store
	vol  []settings.Key
	mute []settings.Key
}

func New() *Module {
	return &Module{}
}

func (*Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Builtin: true, DisplayName: "Audio", DocsOrder: 130, DocsIcon: "cat.audio"}
}

func (*Module) Register(r *module.Registry) error {
	both := settings.User | settings.Project
	for i, b := range Buses {
		r.Setting(settings.Desc{
			ID: VolumeKey(b), Kind: schema.KindFloat64, Default: schema.Float64(1), Title: titles[i] + " Volume",
			Category: settings.CategoryAudio, Order: i * 10, Flags: both, Min: 0, Max: 1, HasMin: true, HasMax: true, Step: 0.05,
		})
		r.Setting(settings.Desc{
			ID: MuteKey(b), Kind: schema.KindBool, Title: "Mute " + titles[i],
			Category: settings.CategoryAudio, Order: i*10 + 1, Flags: both,
		})
	}
	return nil
}

func (m *Module) Init(ctx *module.InitContext) error {
	m.st = ctx.Settings
	if m.st != nil {
		for _, b := range Buses {
			m.vol = append(m.vol, m.st.Key(VolumeKey(b)))
			m.mute = append(m.mute, m.st.Key(MuteKey(b)))
		}
	}
	if ctx.Engine != nil {
		ctx.Engine.SetService(MixerID, m)
	}
	return nil
}

func (m *Module) Volume(bus string) float32 {
	i := slices.Index(Buses, bus)
	if i < 0 || m.st == nil || len(m.vol) == 0 {
		return 1
	}
	if m.st.Bool(m.mute[0]) || m.st.Bool(m.mute[i]) {
		return 0
	}
	v := m.st.Float(m.vol[i])
	if i > 0 {
		v *= m.st.Float(m.vol[0])
	}
	return float32(v)
}

func Of(e *engine.Engine) *Module {
	m, _ := engine.Service[*Module](e, MixerID)
	return m
}
