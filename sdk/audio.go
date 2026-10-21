package sdk

import (
	"aego/asset"
	"aego/modules/audio"
)

type (
	Audio        = audio.Service
	AudioOptions = audio.Options
	AudioEvent   = audio.Event
	AudioBus     = audio.Bus
	Sound        = audio.Sound
	Clip         = audio.ClipRef
)

func AudioOf(ctx *Context) *Audio {
	if ctx == nil || ctx.World == nil {
		return nil
	}
	return audio.Of(ctx.World)
}

func BuiltinClip(name string) (Clip, bool) {
	id, ok := asset.BuiltinAudioNamed(name)
	return Clip{ID: id}, ok
}
