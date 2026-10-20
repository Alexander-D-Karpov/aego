---
id: language/standard-library
parent: language
order: 10
title: Standard library
---
# Standard library

`aego/math` — `Pi Tau`; `Floor Ceil Round Trunc Sqrt Sin Cos Tan Asin Acos Exp Log Abs Sign Min Max Clamp Lerp Atan2 Pow Deg2Rad Rad2Deg`; `AbsInt MinInt MaxInt ClampInt`; Vec2: `Length Normalize Dot Cross Dist Rotate Angle LerpVec2`; Vec3: `Length3 Normalize3 Dot3 Cross3 Dist3 LerpVec3`; Quat: `QuatEuler QuatAxisAngle QuatMul QuatRotate QuatSlerp QuatLookDir`. There is no overloading: Vec3 variants carry a `3` suffix, int variants an `Int` suffix.

`aego/log` — `Debug Info Warn Error(msg string)`. Build messages with `+` and `string(x)`.

`aego/rand` — `Float() float` in `[0,1)`, `Bool() bool`, `Int(min, max int) int` in `[min, max)`, `Range(min, max float) float`, `Seed() int`. The source is the world's seeded generator; a replay with the same seed reproduces the numbers.

`aego/time` — `Delta() float`, `Fixed() float`, `Elapsed() float`, `Frame() int`.

`aego/input` — actions, contexts and raw keys; see [Input](ref:input).

`aego/settings` — player and project settings; see [Settings](ref:settings).

```
import "aego/settings"

shake := Strength * settings.GetFloat("accessibility.screenShake")
settings.SetFloat("audio.bus.music.volume", 0.5)
settings.Apply()
```

| Function | |
|---|---|
| `GetBool GetInt GetFloat GetString(id)` | effective value |
| `SetBool SetInt SetFloat SetString(id, v)`, `Reset(id)`, `Apply() bool`, `Revert()` | staged changes |
| `OverrideBool OverrideInt OverrideFloat OverrideString(id, v)`, `ClearOverride(id)`, `ClearOverrides()` | session values |
| `OpenView() CloseView() ViewOpen()`, `NeedsRestart()`, `Save()` | screen and profile |

`aego/scene` — `Find(path string) NodeRef` from the scene root, `Instantiate(prefab AssetRef<Prefab>, parent NodeRef) NodeRef`, `Destroy(NodeRef)`. New nodes appear at the end of the phase; their scripts receive `Ready` before the next `Update`.

`aego/testing` — available only under `aego test`: `True(cond bool, msg string)`, `False(…)`, `Equal(got, want int)`, `EqualFloat(got, want, eps float)`, `EqualString(got, want string)`, `Fail(msg string)`. A failed assertion stops the test and reports the file and line.

Engine modules add their own host modules (`aego/physics`, `aego/audio`, …); each is documented in its module's reference and in the generated library stubs.
