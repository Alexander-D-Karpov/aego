---
id: audio
try: try/AudioSource2D.aescene
try_select: audio.AudioSource2D
icon: cat.audio
group: audio
group_title: Audio
group_icon: cat.audio
group_order: 57
title: Audio
order: 1
---
# Audio

Sound in Aego is imported clips, played by components, scripts and audio events, and mixed on buses that the player sets in Settings. Sounds are presentation: they never change the game state, and a rewind or a replay is silent.

Quick start: drop a `.wav` into the project, add [Audio Source 2D](ref:audio.AudioSource2D) to a node, set Clip and Play On Start, and press Play.

| Piece | Purpose |
|---|---|
| Clip | an imported `.wav`, `.ogg` or `.mp3` file |
| Audio Source 2D | plays a clip or an audio event from its node |
| Audio Listener 2D | the ears of spatial sound |
| Audio Zone 2D | turns a bus down or up while the listener is inside |
| Audio Event | a data asset: variants, randomness, a voice limit and space |
| [aego/audio](ref:aego/audio) | the script API |

Audio runs during Play and in `aego run`. In edit mode the Inspector previews clips, and the Scene View shows the ranges of spatial sources and the zones.

In Try it, the ball hums while it rolls from wall to wall and clicks when it hits one. The camera carries the listener, so the hum pans and fades as the ball moves. Tick **Replay** on the script to start the hum again. The browser starts sound after the first click on the page.

---
id: audio/clips
icon: file.unknown
parent: audio
order: 1
title: Clips
---
# Clips

`.wav` (8, 16, 24 and 32-bit PCM, 32-bit float) and `.ogg` Vorbis, mono or stereo, are imported as they are. An `.mp3` is converted to `.ogg` on import with `ffmpeg`; without it in `PATH` the import fails with AUD001 and the command to convert the file by hand.

Import settings in the `.meta`:

| Setting | Default | Meaning |
|---|---|---|
| `stream` | `auto` | `auto` streams files over 2 MB or 10 seconds; `on` and `off` force it |
| `loopStart`, `loopEnd` | `-1` | loop points in frames of the file; a wav `smpl` chunk gives the first values |
| `gain` | `1` | volume of the clip, before the source and the bus |
| `bus` | `sfx` | the bus when the player of the clip does not choose one |

A clip that is not streamed is decoded once, at the rate of the output, and shared by every voice that plays it. A streamed clip is decoded while it plays, from the asset in memory; it is the choice for music. Loop points outside the sound give AUD002, and the whole sound loops instead.

The Assets panel draws a waveform for every clip.

## Built-in clips

Four clips ship with the engine and need no files: **Beep**, **Click**, **Hum** and **Melody**. They are in every Clip picker under `engine/audio`, and scripts get them with `audio.Builtin("Beep")`. Hum and Melody have loop points over the whole clip, so they loop without a seam. The Try it scenes on these pages play them.

---
id: audio/events
icon: file.data
parent: audio
order: 2
title: Audio events
---
# Audio events

An audio event is a [data asset](ref:data) of type Audio Event (Create → Data Asset → Audio Event). It names one or more clips and how to play them:

| Field | Meaning |
|---|---|
| `Variants` | clips; one is picked at random on every play |
| `Volume`, `Volume Random` | volume, and how much quieter each play may be |
| `Pitch`, `Pitch Random` | pitch, and how far up or down each play may go, so repeated sounds do not ring the same |
| `Bus`, `Priority` | the bus, and which voices the mixer steals first when it runs out |
| `Max Voices`, `Steal` | at the limit a new play stops the oldest voice of the event, the quietest, or is skipped |
| `Cooldown` | seconds of game time before the event plays again |
| `Spatial` … `Pan Width` | see [Spatial sound](ref:audio/spatial) |
| `Loop`, `Pause With` | loops until stopped; `game` pauses with the world, `never` keeps playing |

Scripts play an event with `audio.PlayEvent(Jump)` from an `@export var Jump AssetRef<AudioEvent>`; an Audio Source 2D plays one when its Event is set, and the event's settings replace the source's playback fields. Voice limits and cooldowns count per world. The random picks come from the audio service's own seed, so a replay picks the same variants, and sound never touches the game's random numbers.

Like every data asset, an event can have a base and override a few fields. An event without variants, or a spatial one with Max Distance not larger than Min Distance, is an error (AUD006).

---
id: audio/spatial
try: try/AudioListener2D.aescene
try_select: audio.AudioListener2D
icon: audio.listener
parent: audio
order: 3
title: Spatial sound
---
# Spatial sound

A spatial sound fades and pans with its distance to the listener. The listener is the first enabled, active [Audio Listener 2D](ref:audio.AudioListener2D) in the hierarchy; another active one later in the hierarchy is ignored (AUD005). Without a listener the camera of the main output listens, and AUD004 reminds you of it on spatial sources; without a camera, the origin.

| Field | Unit | Meaning |
|---|---|---|
| `Min Distance` | world units | full volume up to this distance |
| `Max Distance` | world units | silent from this distance |
| `Rolloff` | curve | volume between them: time 0 is Min Distance, time 1 is Max Distance |
| `Pan Width` | 0..1 | how far a sound at Max Distance to the side is panned |

A keyed Rolloff curve is read as the volume; a preset ease is read as the volume falling away; the default curve drops to a quarter halfway. Pan is the distance to the side divided by Max Distance, times Pan Width.

A sound played from a node follows it; `audio.PlayAt` and `audio.PlayEventAt` stay where they started. Positions are read after the frame's movement, so the sound follows what is drawn.

In Try it, the ball carries the listener and the camera follows it. The box in the middle hums: it is loud and centred when the ball passes it, and fades to the side as the ball rolls away. Change Max Distance or Pan Width of the box in the Inspector and listen to the next pass.

---
id: audio/zones
try: try/AudioZone2D.aescene
try_select: audio.AudioZone2D
icon: audio.zone
parent: audio
order: 4
title: Zones
---
# Zones

While the listener is inside the [Area](ref:audio.AudioZone2D.Area) of an [Audio Zone 2D](ref:audio.AudioZone2D), the zone's Bus is turned to its Gain over Fade Time. The area is a rectangle in the zone node's space.

- Among overlapping zones on one bus the highest Priority wins, then the zone entered later. Leaving one zone leaves the others in place.
- Zones act on the world layer of the mixer, under the player's settings and the session, and end with the world.
- A zone changes the whole bus, not one source: use it for muffling music indoors or quieting ambience near a menu.

In Try it, the music bus drops to a fifth while the listening ball rolls through the purple area, and comes back when it leaves. Set Gain to 0 to silence the music inside, or raise Fade Time for a slower change.

---
id: audio/buses
icon: panel.mixer
parent: audio
order: 5
title: Buses and settings
---
# Buses and settings

Every sound plays on a bus: Music, Effects, Interface, Ambience or Voice, all under Master. The volume of a bus is three layers multiplied:

1. the player's settings, `audio.bus.<name>.volume` and `mute`, on the Audio page of [Settings](ref:settings); a project sets their defaults in `project.aego`;
2. the session, from `audio.SetBusVolume` and `audio.Mute` in scripts and the faders of the Audio Mixer, never saved;
3. the world, from audio zones.

Every change ramps, so nothing clicks. `audio.enabled` turns all sound off without closing the output; `audio.bufferSize` and `audio.sampleRate` reopen it, and playing sounds go on.

---
id: audio/music
icon: play.pause
parent: audio
order: 6
title: Music, pause and rewind
---
# Music, pause and rewind

`audio.Music(Theme, 1.5)` loops a clip on the music bus and crossfades from the music that played before; `audio.StopMusic(1)` fades it out. Music keeps playing while the game is paused.

Pausing the world pauses sounds whose Pause With is `game` and leaves those with `never`; `PlayOptions.Unscaled` does the same for a single play. A sound stops with a short fade when the node that played it is destroyed.

Rewind and replay are silent: sounds started before a seek stop at once, nothing plays while the recording replays, and after Live new sounds play again. Sounds are not part of the state hash, so a game plays back the same with and without them. Wait for game logic with a timer, never with a sound.

---
id: audio/scripting
icon: file.script
parent: audio
order: 7
title: Scripting
---
# Scripting

    import "aego/audio"

    @export var Jump AssetRef<AudioEvent>
    @export var Theme AssetRef<Clip>

    func Ready() {
      audio.Music(Theme, 1.5)
    }

    func OnJump() {
      audio.PlayEvent(Jump)
    }

| Function | |
|---|---|
| `Play(clip) Sound`, `PlayWith(clip, options) Sound` | play a clip on its bus, or with `PlayOptions` |
| `PlayAt(clip, position) Sound` | play a clip at a world position, spatial |
| `PlayEvent(event) Sound`, `PlayEventAt(event, position) Sound` | play an audio event; a spatial event follows the node of the script |
| `Music(clip, seconds) Sound`, `StopMusic(seconds)` | loop music with a crossfade, fade it out |
| `BusVolume(bus) float`, `SetBusVolume(bus, volume)`, `Mute(bus, on)` | session volume of a bus, on top of the player's settings |
| `Pause(on)` | pause or resume every sound of this world |
| `Builtin(name) AssetRef<Clip>` | a built-in clip: Beep, Click, Hum or Melody |

`PlayOptions` has Volume, Pitch, Pan, Bus, Loop, FadeIn, Priority and Unscaled; Volume and Pitch 0 mean 1, and Bus Master plays on the bus of the clip. `Sound` has `Done()`, `Playing()`, `Stop()`, `StopFade(seconds)`, `Pause()`, `Resume()`, `SetVolume`, `SetPitch`, `SetPan`, `Position()` and the signal `Completed(finished bool)`; `yield` waits for it, and a nil sound is done. Every play belongs to the node of the script and stops when the node is destroyed.

Component methods: `AudioSource2D.Play()`, `Stop()`, `StopFade(seconds)` and `IsPlaying()`, and the signals `Started` and `Finished(finished bool)`.

```aes run
package demo

import (
    "aego/audio"
    "aego/log"
)

script Chime {
    var hum audio.Sound
    var frames int

    func Ready() {
        hum = audio.PlayWith(audio.Builtin("Hum"), audio.PlayOptions{Volume: 0.5, Pitch: 1.5, Loop: true})
        hum.Completed.Connect(Ended)
        log.Info("the hum plays: " + string(hum.Playing()))
    }

    func Ended(finished bool) {
        log.Info("the hum ended, finished " + string(finished))
    }

    func Update(dt float) {
        frames++
        if frames == 60 {
            hum.StopFade(0.2)
            log.Info("fading at " + string(hum.Position()) + " s")
        }
    }
}
```

---
id: audio/editor
icon: tool.move
parent: audio
order: 8
title: Editor tools
---
# Editor tools

- **Audio Mixer** (Window → Audio Mixer, or the Audio workspace) shows a meter, a fader, mute and solo for every bus, the voices and the underruns. Faders change the session; **Reset Session** sets them back, **Save to Profile** writes them into the player's settings.
- **Inspector.** In edit mode Audio Source 2D has **Preview** and **Stop** for its clip, with a progress bar. During Play it has **Play** and **Stop**, and shows how far the sound is.
- **Handles.** Select a spatial source to drag Min Distance and Max Distance, a zone to drag its area. Every drag is one undo step.
- **Audio → Audio Ranges** turns the circles and zones on and off in the Scene View and in the Game View during Play, where the listener, the playing spatial voices and lines to them are drawn over the game.
- **Events** lists every play and stop as `audio`; **Profiler** has an Audio section with the output, the voices, the mix time and the underruns.
- **Problems** lists sources with neither a clip nor an event (AUD003), spatial sources without a listener (AUD004), extra listeners (AUD005), broken events (AUD006), unreadable files (AUD001) and loop points outside the sound (AUD002).

---
id: audio/web
icon: view.fullscreen
parent: audio
order: 9
title: Audio on the web
---
# Audio on the web

Browsers start sound only after a click, a key or a touch on the page; a page opened by a click, such as Try it on this site, plays at once. A web export shows a mute button in the corner, and the browser remembers the choice. When the page is served with `Cross-Origin-Opener-Policy: same-origin` and `Cross-Origin-Embedder-Policy: require-corp`, the output has one block less latency. Streamed clips are read from the downloaded data, never over the network.

---
id: audio/go
icon: code.brackets
parent: audio
order: 10
title: Go
---
# Go

    snd := sdk.AudioOf(ctx)
    beep, _ := sdk.BuiltinClip("Beep")
    s := snd.PlayAt(beep.ID, pos, sdk.AudioOptions{Volume: 0.8}, node)
    if snd.Done(s) { ... }

`Service` has `Play`, `PlayAt`, `PlayEvent`, `PlayEventAt`, `Trigger` and `TriggerAt` for an event value built in Go, `Music`, `StopMusic`, `Stop`, `Pause`, `SetVolume`, `SetPitch`, `SetPan`, `Done`, `Playing`, `Position` and `PauseAll`. Each play world has its own service; the module, found with `audio.ModuleOf(engine)`, owns the mixer, the session volumes and the clip cache.

The system `audio.ready` (PreUpdate) starts Play On Start sources on the first frame; `audio.world` (PostUpdate, also while paused) moves spatial voices, follows the listener and applies zones. The mixer runs on its own thread and takes commands through a ring buffer, so the game thread never waits for it.
