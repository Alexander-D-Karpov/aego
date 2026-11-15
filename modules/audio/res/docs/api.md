---
id: aego/audio.PlayOptions.Bus
---
Selects the playback bus. Master here means the clip's configured bus, not a forced routing to the master bus.

---
id: aego/audio.PlayOptions.FadeIn
---
Sets the fade-in duration in seconds; zero starts without a fade.

---
id: aego/audio.PlayOptions.Loop
---
Repeats the clip until stopped instead of finishing after one playback.

---
id: aego/audio.PlayOptions.Pan
---
Sets stereo balance from −1 (left) through 0 (centre) to 1 (right).

---
id: aego/audio.PlayOptions.Pitch
---
Scales playback speed and pitch together; 1 is the original speed and 0 also selects 1 for these options.

---
id: aego/audio.PlayOptions.Priority
---
Sets voice priority, clamped to 0–255. Higher-priority sounds are preferred when the mixer must replace a voice.

---
id: aego/audio.PlayOptions.Unscaled
---
Keeps this sound playing while the game is paused when true; false ties it to game pause.

---
id: aego/audio.PlayOptions.Volume
---
Multiplies the clip's gain; 1 is unchanged and 0 also selects 1 for these options, rather than muting the sound.

---
id: aego/data.AudioEvent.Bus
---
Routes voices started by this event to the selected audio bus.

---
id: aego/data.AudioEvent.Cooldown
---
Sets the minimum interval between accepted triggers of this event in seconds; zero disables the interval limit.

---
id: aego/data.AudioEvent.Loop
---
Repeats each started voice until it is stopped, including any selected clip variant.

---
id: aego/data.AudioEvent.MaxDistance
---
Sets the outer attenuation distance in world units. For spatial audio it must exceed MinDistance.

---
id: aego/data.AudioEvent.MaxVoices
---
Limits simultaneous voices of this event; zero imposes no event-specific limit. The mixer still has its own voice budget.

---
id: aego/data.AudioEvent.MinDistance
---
Sets the inner distance in world units before spatial distance attenuation begins.

---
id: aego/data.AudioEvent.PanWidth
---
Scales spatial stereo panning from 0 (centred) to 1 (full width); it does not change distance attenuation.

---
id: aego/data.AudioEvent.PauseWith
---
Chooses Game to pause voices with the game, or Never to keep them playing during game pause.

---
id: aego/data.AudioEvent.Pitch
---
Sets base playback speed and pitch together, from 0.25 to 4; 1 preserves the clip's original speed.

---
id: aego/data.AudioEvent.PitchRand
---
Sets the random pitch fraction from 0 to 1. Each trigger multiplies Pitch by a random factor between 1−PitchRand and 1+PitchRand.

---
id: aego/data.AudioEvent.Priority
---
Sets mixer voice priority from 0 to 255; higher-priority sounds are preferred under voice pressure.

---
id: aego/data.AudioEvent.Spatial
---
Enables listener-relative distance attenuation and panning instead of nonspatial playback.

---
id: aego/data.AudioEvent.Steal
---
Chooses whether reaching MaxVoices rejects the new trigger or replaces an existing voice by age or quietness.

---
id: aego/data.AudioEvent.Volume
---
Sets base linear gain from 0 to 2 before event randomisation and the clip and bus gains; zero is silent.

---
id: aego/data.AudioEvent.VolumeRand
---
Sets the random gain reduction fraction from 0 to 1. Each trigger uses a gain between Volume×(1−VolumeRand) and Volume.

---
id: aego/audio
summary: Plays clips, audio events and music, and sets bus volumes for the session.
---

Sounds are presentation: they never change the game state, and a rewind or replay is silent. Wait for game logic with a timer, not with a `Sound`. The engine ships four clips for tests and sketches, `audio.Builtin("Beep")`, `Click`, `Hum` and `Melody`; the last two loop without a seam. The example plays a hum and music, then fades both out. See [Audio](ref:audio).

## Example

```aes run
package demo

import (
    "aego/audio"
    "aego/log"
)

script Chime {
    var tone audio.Sound
    var frames int

    func Ready() {
        tone = audio.PlayWith(audio.Builtin("Hum"), audio.PlayOptions{Volume: 0.5, Pitch: 1.5, Loop: true})
        audio.Music(audio.Builtin("Melody"), 0.5)
        audio.SetBusVolume(audio.Bus.Music, 0.6)
        log.Info("the hum plays: " + string(tone.Playing()))
    }

    func Update(dt float) {
        frames++
        if frames == 60 {
            tone.StopFade(0.2)
            audio.StopMusic(0.5)
            log.Info("the hum fades out: " + string(!tone.Playing()))
        }
    }
}
```

---
id: aego/data
summary: Data asset types that scripts read through AssetRef and TableRef.
---

A data type registered by a module or a project appears here as a struct. A script field `AssetRef<T>` holds a record, `TableRef<T>` a table; fields are read through the reference. Module data types carry the module's name, such as `AudioEvent`, so they do not collide with a game's own types. See [Data assets](ref:data).

## Example

```aes run
package demo

import (
    "aego/audio"
    "aego/log"
)

script Jumper {
    @export var Jump AssetRef<AudioEvent>

    func Ready() {
        if Jump == nil {
            log.Info("no jump event set")
            return
        }
        log.Info("jump volume " + string(Jump.Volume))
        audio.PlayEvent(Jump)
    }
}
```
