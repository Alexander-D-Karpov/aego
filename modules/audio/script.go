package audio

import (
	"aego/asset"
	"aego/core/mathx"
	"aego/data"
	"aego/script/abi"
	"aego/script/bind"
)

func live(c *bind.Ctx) *Service {
	if c.World == nil {
		return nil
	}
	return Of(c.World)
}

func need(c *bind.Ctx) *Service {
	s := live(c)
	if s == nil {
		c.Fail("audio plays only during Play")
	}
	return s
}

func installScript(m *bind.Module, mod *Module) {
	bind.Enum[Bus](m, "Bus", bind.Member("Master", BusMaster), bind.Member("Music", BusMusic), bind.Member("Sfx", BusSfx),
		bind.Member("UI", BusUI), bind.Member("Ambience", BusAmbience), bind.Member("Voice", BusVoice))
	bind.Enum[Steal](m, "Steal", bind.Member("Oldest", StealOldest), bind.Member("Quietest", StealQuietest), bind.Member("None", StealNone))
	bind.Enum[PauseWith](m, "PauseWith", bind.Member("Game", PauseGame), bind.Member("Never", PauseNever))
	bind.Describe(m, "Bus", bind.Doc("Bus is a mixer bus: Master, Music, Sfx, UI, Ambience or Voice."))
	bind.Describe(m, "Steal", bind.Doc("Steal is what an audio event does at Max Voices: stop the oldest voice, stop the quietest, or skip the new play."))
	bind.Describe(m, "PauseWith", bind.Doc("PauseWith decides whether a sound pauses with the game or keeps playing."))
	bind.Struct[Options](m, "PlayOptions", bind.Doc("PlayOptions tune one play. Volume and Pitch 0 mean 1; Bus Master plays on the bus of the clip; Unscaled keeps playing while the game is paused."))
	bind.Handle(m, "Sound", true, soundOf, Sound.key,
		bind.Doc("Sound is a playing sound. yield s waits until it ends; a nil or finished sound is done. It is a presentation event: wait for game logic with a timer, not a sound."))

	bind.Func1(m, "Play", func(c *bind.Ctx, clip ClipRef) Sound {
		if s := need(c); s != nil {
			return s.Play(clip.ID, Options{}, c.Node)
		}
		return 0
	}, bind.Doc("Play plays a clip on its bus. The sound stops when the node of the script is destroyed."), bind.Params("clip"), bind.Example("audio.Play(Coin)"))
	bind.Func2(m, "PlayWith", func(c *bind.Ctx, clip ClipRef, o Options) Sound {
		if s := need(c); s != nil {
			return s.Play(clip.ID, o, c.Node)
		}
		return 0
	}, bind.Doc("PlayWith plays a clip with options."), bind.Params("clip", "options"),
		bind.Example("audio.PlayWith(Step, audio.PlayOptions{Volume: 0.5, Pitch: 1.1})"))
	bind.Func2(m, "PlayAt", func(c *bind.Ctx, clip ClipRef, p mathx.Vec2) Sound {
		if s := need(c); s != nil {
			return s.PlayAt(clip.ID, p, Options{}, c.Node)
		}
		return 0
	}, bind.Doc("PlayAt plays a clip at a world position, fading and panning with the distance to the listener."), bind.Params("clip", "position"))
	bind.Func1(m, "PlayEvent", func(c *bind.Ctx, ev data.Ref[Event]) Sound {
		if s := need(c); s != nil {
			return s.PlayEvent(ev.ID, c.Node)
		}
		return 0
	}, bind.Doc("PlayEvent plays an audio event; a spatial event follows the node of the script."), bind.Params("event"), bind.Example("audio.PlayEvent(Jump)"))
	bind.Func2(m, "PlayEventAt", func(c *bind.Ctx, ev data.Ref[Event], p mathx.Vec2) Sound {
		if s := need(c); s != nil {
			return s.PlayEventAt(ev.ID, p, c.Node)
		}
		return 0
	}, bind.Doc("PlayEventAt plays an audio event at a world position."), bind.Params("event", "position"))
	bind.Func2(m, "Music", func(c *bind.Ctx, clip ClipRef, fade float32) Sound {
		if s := need(c); s != nil {
			return s.Music(clip.ID, fade)
		}
		return 0
	}, bind.Doc("Music loops a clip on the music bus, crossfading from the music that played over seconds."), bind.Params("clip", "seconds"),
		bind.Example("audio.Music(Theme, 1.5)"))
	bind.Proc1(m, "StopMusic", func(c *bind.Ctx, fade float32) {
		if s := live(c); s != nil {
			s.StopMusic(fade)
		}
	}, bind.Doc("StopMusic fades the music out over seconds."), bind.Params("seconds"))
	bind.Func1(m, "BusVolume", func(c *bind.Ctx, b Bus) float32 {
		return mod.BusVolume(b)
	}, bind.Doc("BusVolume returns the session volume of a bus set by SetBusVolume; the player's settings are separate."), bind.Params("bus"))
	bind.Proc2(m, "SetBusVolume", func(c *bind.Ctx, b Bus, v float32) {
		mod.SetBusVolume(b, v)
	}, bind.Doc("SetBusVolume sets the session volume of a bus, on top of the player's settings; it is not saved."), bind.Params("bus", "volume"))
	bind.Proc2(m, "Mute", func(c *bind.Ctx, b Bus, on bool) {
		mod.SetBusMuted(b, on)
	}, bind.Doc("Mute silences a bus for this session."), bind.Params("bus", "on"))
	bind.Func1(m, "Builtin", func(c *bind.Ctx, name string) ClipRef {
		id, ok := asset.BuiltinAudioNamed(name)
		if !ok {
			c.Fail("audio: there is no built-in clip " + name + "; the built-in clips are Beep, Click, Hum and Melody")
			return ClipRef{}
		}
		return ClipRef{ID: id}
	}, bind.Doc("Builtin returns a clip that ships with the engine: Beep, Click, Hum (loopable) or Melody (loopable)."),
		bind.Params("name"), bind.Example(`audio.Play(audio.Builtin("Beep"))`))
	bind.Proc1(m, "Pause", func(c *bind.Ctx, on bool) {
		if s := live(c); s != nil {
			s.PauseAll(on)
		}
	}, bind.Doc("Pause pauses or resumes every sound of this world."), bind.Params("on"))

	hm := m.Registry().HandleMethods(KindSound)
	bind.Func1(hm, "Done", func(c *bind.Ctx, h Sound) bool {
		s := live(c)
		return s == nil || s.Done(h)
	}, bind.Doc("Done reports whether the sound ended, was stopped or is nil."))
	bind.Func1(hm, "Playing", func(c *bind.Ctx, h Sound) bool {
		s := live(c)
		return s != nil && s.Playing(h)
	}, bind.Doc("Playing reports whether the sound is audible now: not done, not stopping and not paused."))
	bind.Proc1(hm, "Stop", func(c *bind.Ctx, h Sound) {
		if s := live(c); s != nil {
			s.Stop(h, 0)
		}
	}, bind.Doc("Stop ends the sound with a short fade."))
	bind.Proc2(hm, "StopFade", func(c *bind.Ctx, h Sound, sec float32) {
		if s := live(c); s != nil {
			s.Stop(h, sec)
		}
	}, bind.Doc("StopFade fades the sound out over seconds."), bind.Params("seconds"))
	bind.Proc1(hm, "Pause", func(c *bind.Ctx, h Sound) {
		if s := live(c); s != nil {
			s.Pause(h, true)
		}
	}, bind.Doc("Pause holds the sound where it is."))
	bind.Proc1(hm, "Resume", func(c *bind.Ctx, h Sound) {
		if s := live(c); s != nil {
			s.Pause(h, false)
		}
	}, bind.Doc("Resume continues a paused sound."))
	bind.Proc2(hm, "SetVolume", func(c *bind.Ctx, h Sound, v float32) {
		if s := live(c); s != nil {
			s.SetVolume(h, v)
		}
	}, bind.Doc("SetVolume changes the volume with a short ramp."), bind.Params("volume"))
	bind.Proc2(hm, "SetPitch", func(c *bind.Ctx, h Sound, p float32) {
		if s := live(c); s != nil {
			s.SetPitch(h, p)
		}
	}, bind.Doc("SetPitch changes speed and pitch, 0.25 to 4."), bind.Params("pitch"))
	bind.Proc2(hm, "SetPan", func(c *bind.Ctx, h Sound, p float32) {
		if s := live(c); s != nil {
			s.SetPan(h, p)
		}
	}, bind.Doc("SetPan pans a sound that is not spatial, -1 to 1."), bind.Params("pan"))
	bind.Func1(hm, "Position", func(c *bind.Ctx, h Sound) float32 {
		if s := live(c); s != nil {
			return s.Position(h)
		}
		return 0
	}, bind.Doc("Position returns the seconds played, within the loop for a looping sound."))
	bind.Signal(hm, "Completed", []abi.TypeRef{abi.T(abi.Bool)}, bind.Params("finished"),
		bind.Doc("Completed runs once when the sound ends: finished is false after Stop, when it was cut off, or when its node died."))
}
