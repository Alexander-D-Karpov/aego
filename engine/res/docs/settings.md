---
id: settings
title: Settings
order: 18
---
# Settings

One registry holds every option a player or a project can change: window mode, volume, bindings, accessibility. Each setting belongs to a module and has an id `<module>.<path>`, like `display.window.mode` or `audio.bus.music.volume`.

## Where a value comes from

| Layer | Set by | Saved in |
|---|---|---|
| default | the module | code |
| project | Project Settings → Defaults | `project.aego`, block `defaults {}` |
| user | the player, the settings screen, scripts | the player profile |
| session | `settings.Override*` | never saved |

The last layer that has a value wins. A cutscene can lower the music with `OverrideFloat` and `ClearOverride` it afterwards; the player's choice is untouched.

## The player profile

`<user config>/aego/<module>/profiles/<name>/settings.aetext`; `aego run --profile test` and `aego edit --profile test` use another one. Play in the editor shares the profile with the game. A damaged file is kept as `settings.aetext.bad` and the game starts with defaults. Values of modules the game does not load are kept as they are.

## The settings screen

F10 in Play and in `aego run` opens it, or `settings.OpenView()` from a script; the game is paused while it is open. Apply sets every changed value at once: if one cannot be applied, none change. Window mode, monitor and resolution ask for confirmation and go back after 15 seconds without it.

`display.renderScale` draws the game into an offscreen image of window size × scale and stretches it to the window in `aego run`; below 1 it trades sharpness for speed, above 1 it supersamples. The settings screen is drawn at full resolution over that image. The editor's Game View always renders at scale 1.

## Scripts

    import "aego/settings"

    shake := Strength * settings.GetFloat("accessibility.screenShake")
    settings.SetFloat("audio.bus.music.volume", 0.5)
    settings.Apply()

| Function | |
|---|---|
| `GetBool GetInt GetFloat GetString(id)` | effective value |
| `SetBool SetInt SetFloat SetString(id, v)`, `Reset(id)`, `Apply() bool`, `Revert()` | staged changes |
| `OverrideBool OverrideInt OverrideFloat OverrideString(id, v)`, `ClearOverride(id)`, `ClearOverrides()` | session values |
| `OpenView() CloseView() ViewOpen()`, `NeedsRestart()`, `Save()` | screen and profile |

A script reacts to changes with an event method:

    func OnSettingChanged(id string) {
      if id == "accessibility.reducedMotion" {
        reduced = settings.GetBool(id)
      }
    }

It runs once per changed setting after Apply, Revert, an override or a confirmation timeout, in the same frame for changes made from scripts.

## Go

    sdk.Setting(r, sdk.SettingDesc{
      ID: "mygame.difficulty", Kind: schema.KindEnum, Category: "Gameplay",
      Flags: sdk.SettingUser | sdk.SettingProject,
      Choices: []sdk.SettingChoice{{Name: "easy", Value: schema.Enum(0)}, {Name: "hard", Value: schema.Enum(1)}},
    })
    st := sdk.Settings(ctx)
    k := st.Key("mygame.difficulty")
    hard := st.Int(k) == 1

Read through a `Key` in frame code; it does not allocate.

## Rules for modules

- Camera shake multiplies its strength by `accessibility.screenShake`.
- With `accessibility.reducedMotion` on, cameras, particles and transitions cut large or fast motion.
- A quality option registers `Presets` per preset name; `display.preset` sets them, and a manual change makes the preset Custom.
