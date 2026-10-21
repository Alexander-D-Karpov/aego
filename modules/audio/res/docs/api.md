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
