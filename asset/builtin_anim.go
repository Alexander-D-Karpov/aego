package asset

import (
	"math"
	"strconv"
	"strings"
	"sync"

	"aego/core/ids"
	"aego/gpu"
	"aego/render/texture"
)

const (
	runnerSize  = 16
	runnerFirst = 0xb00
	runnerCount = 8
)

var (
	BuiltinRunClip = ids.AssetID{Hi: builtinHi, Lo: 0xb10}
	BuiltinBobClip = ids.AssetID{Hi: builtinHi, Lo: 0xb11}
	// BuiltinJumpClip holds the runner's stride pose while it is in the air.
	BuiltinJumpClip = ids.AssetID{Hi: builtinHi, Lo: 0xb12}
	// BuiltinHitClip swings the node once, 0.3 s, for the platformer controller's Hit state.
	BuiltinHitClip = ids.AssetID{Hi: builtinHi, Lo: 0xb13}
)

var BuiltinPlatformerCtrl = ids.AssetID{Hi: builtinHi, Lo: 0xb20}

func BuiltinRunner(i int) ids.AssetID {
	return ids.AssetID{Hi: builtinHi, Lo: runnerFirst + uint64(i%runnerCount)}
}

var runnerData [runnerCount]struct {
	once sync.Once
	data *texture.Data
}

func BuiltinTexture(id ids.AssetID) (*texture.Data, bool) {
	switch {
	case id == BuiltinParticlePuff:
		return particlePuff(), true
	case id == BuiltinParticleStreak:
		return particleStreak(), true
	case id == BuiltinMaterialNoise:
		return builtinNoiseTexture(), true
	case id == BuiltinWhite:
		return &texture.Data{Width: 1, Height: 1, Format: gpu.TexSRGBA8, Pixels: []byte{255, 255, 255, 255}, Premultiplied: true}, true
	case id.Hi == builtinHi && id.Lo >= runnerFirst && id.Lo < runnerFirst+runnerCount:
		k := id.Lo - runnerFirst
		c := &runnerData[k]
		c.once.Do(func() { c.data = runnerFrame(int(k)) })
		return c.data, true
	}
	return nil, false
}

func runnerFrame(i int) *texture.Data {
	const n = runnerSize
	px := make([]byte, n*n*4)
	body := [4]byte{236, 236, 236, 255}
	eye := [4]byte{24, 28, 36, 255}
	set := func(x, y int, c [4]byte) {
		if x >= 0 && y >= 0 && x < n && y < n {
			copy(px[(y*n+x)*4:], c[:])
		}
	}
	ph := float64(i) / runnerCount * 2 * math.Pi
	bob := int(math.Round(math.Abs(math.Sin(ph))))
	for y := 3; y <= 9; y++ {
		for x := 4; x <= 11; x++ {
			if (y == 3 || y == 9) && (x == 4 || x == 11) {
				continue
			}
			set(x, y-bob, body)
		}
	}
	set(9, 5-bob, eye)
	set(10, 5-bob, eye)
	for leg, off := range [2]float64{0, math.Pi} {
		hip := 6 + leg*3
		foot := hip + int(math.Round(math.Sin(ph+off)*2.5))
		for y := 10 - bob; y <= 14; y++ {
			t := float64(y-(10-bob)) / float64(4+bob)
			set(hip+int(math.Round(float64(foot-hip)*t)), y, body)
		}
	}
	return &texture.Data{Width: n, Height: n, Format: gpu.TexSRGBA8, Pixels: px, Premultiplied: true}
}

func assetLit(id ids.AssetID) string {
	return `asset("` + id.String() + `")`
}

func runClipText() string {
	var b strings.Builder
	b.WriteString("anim version=1 length=0.8 loop=loop {\n  frames \"\" fps=10 {\n    sprites=[")
	for i := range runnerCount {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(assetLit(BuiltinRunner(i)))
	}
	b.WriteString("]\n  }\n  event t=0.2 name=\"Footstep\" mode=presentation {}\n  event t=0.6 name=\"Footstep\" mode=presentation {}\n}\n")
	return b.String()
}

const bobClipText = `anim version=1 length=1 loop=loop {
  track "" prop="transform.rotation" kind=float32 authority=simulation {
    key t=0 v=-0.2 {}
    key t=0.5 v=0.2 {}
    key t=1 v=-0.2 {}
  }
  event t=0.5 name="Hit" payload=1 {}
}
`

func jumpClipText() string {
	return "anim version=1 length=0.1 loop=once {\n  frames \"\" fps=10 {\n    sprites=[" + assetLit(BuiltinRunner(2)) + "]\n  }\n}\n"
}

const hitClipText = `anim version=1 length=0.3 loop=once {
  track "" prop="transform.rotation" kind=float32 authority=simulation {
    key t=0 v=0 {}
    key t=0.08 v=-0.5 {}
    key t=0.18 v=0.25 {}
    key t=0.3 v=0 {}
  }
}
`

func BuiltinAnimText(id ids.AssetID) (string, bool) {
	switch id {
	case BuiltinRunClip:
		return runClipText(), true
	case BuiltinBobClip:
		return bobClipText, true
	case BuiltinJumpClip:
		return jumpClipText(), true
	case BuiltinHitClip:
		return hitClipText, true
	}
	return "", false
}

func platformerCtrlText() string {
	return `controller version=1 {
  entry="Idle"
  param "Speed" kind=float {}
  param "Grounded" kind=bool default=true {}
  param "Attack" kind=trigger {}
  state "Idle" {}
  state "Run" clip=` + assetLit(BuiltinRunClip) + ` {}
  state "Jump" clip=` + assetLit(BuiltinJumpClip) + ` {}
  state "Hit" clip=` + assetLit(BuiltinHitClip) + ` {}
  transition "Idle" to="Run" duration=0.1 {
    cond "Speed" op=gt value=0.1 {}
  }
  transition "Run" to="Idle" duration=0.1 {
    cond "Speed" op=le value=0.1 {}
  }
  transition "any" to="Jump" interrupt=true {
    cond "Grounded" op=eq value=false {}
  }
  transition "Jump" to="Idle" duration=0.05 {
    cond "Grounded" {}
  }
  transition "any" to="Hit" interrupt=true {
    cond "Attack" {}
  }
  transition "Hit" to="Idle" exitTime=1 duration=0.05 {}
}
`
}

func BuiltinCtrlText(id ids.AssetID) (string, bool) {
	if id == BuiltinPlatformerCtrl {
		return platformerCtrlText(), true
	}
	return "", false
}

func builtinAnimEntries() []Builtin {
	out := make([]Builtin, 0, runnerCount+5)
	for i := range runnerCount {
		out = append(out, Builtin{ID: BuiltinRunner(i), Name: "engine/sprites/runner_" + strconv.Itoa(i), Kind: ArtTexture})
	}
	return append(out,
		Builtin{ID: BuiltinRunClip, Name: "engine/anim/run", Kind: ArtAnim},
		Builtin{ID: BuiltinBobClip, Name: "engine/anim/bob", Kind: ArtAnim},
		Builtin{ID: BuiltinJumpClip, Name: "engine/anim/jump", Kind: ArtAnim},
		Builtin{ID: BuiltinHitClip, Name: "engine/anim/hit", Kind: ArtAnim},
		Builtin{ID: BuiltinPlatformerCtrl, Name: "engine/anim/platformer", Kind: ArtCtrl})
}
