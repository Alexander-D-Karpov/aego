package asset

import (
	"embed"
	"strings"
	"sync"

	"aego/core/ids"
	"aego/gpu"
	"aego/render/texture"
)

// These IDs are persistent project references, independent of registration order.
var (
	BuiltinMaterialUnlit     = ids.AssetID{Hi: builtinHi, Lo: 0xc00}
	BuiltinMaterialAdditive  = ids.AssetID{Hi: builtinHi, Lo: 0xc01}
	BuiltinMaterialFlash     = ids.AssetID{Hi: builtinHi, Lo: 0xc02}
	BuiltinMaterialDissolve  = ids.AssetID{Hi: builtinHi, Lo: 0xc03}
	BuiltinMaterialOutline   = ids.AssetID{Hi: builtinHi, Lo: 0xc04}
	BuiltinMaterialPixelSnap = ids.AssetID{Hi: builtinHi, Lo: 0xc05}
	BuiltinMaterialLit       = ids.AssetID{Hi: builtinHi, Lo: 0xc06}
	BuiltinMaterialLitMesh2D = ids.AssetID{Hi: builtinHi, Lo: 0xc07}
	BuiltinMaterialPostTint  = ids.AssetID{Hi: builtinHi, Lo: 0xc08}
	BuiltinShaderPostTint    = ids.AssetID{Hi: builtinHi, Lo: 0xc88}
	BuiltinMaterialNoise     = ids.AssetID{Hi: builtinHi, Lo: 0xc40}
	BuiltinShaderUnlit       = ids.AssetID{Hi: builtinHi, Lo: 0xc80}
	BuiltinShaderAdditive    = ids.AssetID{Hi: builtinHi, Lo: 0xc81}
	BuiltinShaderFlash       = ids.AssetID{Hi: builtinHi, Lo: 0xc82}
	BuiltinShaderDissolve    = ids.AssetID{Hi: builtinHi, Lo: 0xc83}
	BuiltinShaderOutline     = ids.AssetID{Hi: builtinHi, Lo: 0xc84}
	BuiltinShaderPixelSnap   = ids.AssetID{Hi: builtinHi, Lo: 0xc85}
	BuiltinShaderLit         = ids.AssetID{Hi: builtinHi, Lo: 0xc86}
	BuiltinShaderLitMesh2D   = ids.AssetID{Hi: builtinHi, Lo: 0xc87}
)

type builtinMaterialDef struct {
	name             string
	material, shader ids.AssetID
}

var builtinMaterialDefs = [...]builtinMaterialDef{
	{"Unlit", BuiltinMaterialUnlit, BuiltinShaderUnlit},
	{"Additive", BuiltinMaterialAdditive, BuiltinShaderAdditive},
	{"Flash", BuiltinMaterialFlash, BuiltinShaderFlash},
	{"Dissolve", BuiltinMaterialDissolve, BuiltinShaderDissolve},
	{"Outline", BuiltinMaterialOutline, BuiltinShaderOutline},
	{"PixelSnap", BuiltinMaterialPixelSnap, BuiltinShaderPixelSnap},
	{"Lit", BuiltinMaterialLit, BuiltinShaderLit},
	{"LitMesh2D", BuiltinMaterialLitMesh2D, BuiltinShaderLitMesh2D},
	{"PostTint", BuiltinMaterialPostTint, BuiltinShaderPostTint},
}

//go:embed res/materials
var builtinMaterialFiles embed.FS

func BuiltinShader(id ids.AssetID) ([]byte, bool) {
	for _, d := range builtinMaterialDefs {
		if id == d.shader {
			b, err := builtinMaterialFiles.ReadFile("res/materials/" + strings.ToLower(d.name) + ".aesh")
			return b, err == nil
		}
	}
	return nil, false
}
func BuiltinMaterial(id ids.AssetID) ([]byte, bool) {
	for _, d := range builtinMaterialDefs {
		if id == d.material {
			b, err := builtinMaterialFiles.ReadFile("res/materials/" + strings.ToLower(d.name) + ".aemt")
			return b, err == nil
		}
	}
	return nil, false
}
func BuiltinMaterialNamed(name string) (ids.AssetID, bool) {
	if strings.EqualFold(name, "Default") {
		return BuiltinMaterialUnlit, true
	}
	for _, d := range builtinMaterialDefs {
		if strings.EqualFold(name, d.name) {
			return d.material, true
		}
	}
	return ids.AssetID{}, false
}
func BuiltinShaderNamed(name string) (ids.AssetID, bool) {
	if strings.EqualFold(name, "Default") {
		return BuiltinShaderUnlit, true
	}
	for _, d := range builtinMaterialDefs {
		if strings.EqualFold(name, d.name) {
			return d.shader, true
		}
	}
	return ids.AssetID{}, false
}
func BuiltinMaterialNames() []string {
	out := make([]string, len(builtinMaterialDefs))
	for i, d := range builtinMaterialDefs {
		out[i] = d.name
	}
	return out
}
func BuiltinShaderNames() []string { return BuiltinMaterialNames() }

func builtinMaterialEntries() []Builtin {
	out := make([]Builtin, 0, len(builtinMaterialDefs)*2+1)
	for _, d := range builtinMaterialDefs {
		name := strings.ToLower(d.name)
		out = append(out, Builtin{ID: d.material, Name: "engine/materials/" + name, Kind: ArtMaterial}, Builtin{ID: d.shader, Name: "engine/shaders/" + name, Kind: ArtShader})
	}
	return append(out, Builtin{ID: BuiltinMaterialNoise, Name: "engine/textures/material-noise", Kind: ArtTexture})
}

var materialNoise struct {
	once sync.Once
	data *texture.Data
}

func builtinNoiseTexture() *texture.Data {
	materialNoise.once.Do(func() {
		const size = 64
		px := make([]byte, size*size*4)
		state := uint32(0x9e3779b9)
		for i := 0; i < size*size; i++ {
			state ^= state << 13
			state ^= state >> 17
			state ^= state << 5
			v := byte(state >> 24)
			px[i*4], px[i*4+1], px[i*4+2], px[i*4+3] = v, v, v, 255
		}
		materialNoise.data = &texture.Data{Width: size, Height: size, Format: gpu.TexRGBA8, Pixels: px, NonColor: true}
	})
	return materialNoise.data
}
