package asset

import (
	"encoding/binary"
	"math"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/gpu"
)

const (
	magicTexture = "AETX"
	magicRegion  = "AERG"

	textureVersion = 3
	regionVersion  = 4

	textureHeaderSize = 20
	regionSize        = 88
)

// TextureSemantic describes imported pixels: only color is premultiplied; emission RGB is straight and may be sRGB.
// Normal RGB is tangent +Y up in source-image coordinates; mask and height channels are linear data.
type TextureSemantic uint8

const (
	TextureColor TextureSemantic = iota
	TextureData
	TextureNormal
	TextureMask
	TextureHeight
	TextureEmissionRGB
	TextureEmissionMask
	TextureCookieMask
)

type TextureArtifact struct {
	Width, Height int
	Format        gpu.TextureFormat
	Filter        gpu.FilterMode
	Mips          bool
	Semantic      TextureSemantic
	Pixels        []byte
}

func EncodeTexture(t *TextureArtifact) []byte {
	out := make([]byte, textureHeaderSize+len(t.Pixels))
	copy(out[0:4], magicTexture)
	binary.LittleEndian.PutUint32(out[4:8], textureVersion)
	binary.LittleEndian.PutUint32(out[8:12], uint32(t.Width))
	binary.LittleEndian.PutUint32(out[12:16], uint32(t.Height))
	out[16] = byte(t.Format)
	out[17] = byte(t.Filter)
	if t.Mips {
		out[18] = 1
	}
	out[19] = byte(t.Semantic)
	copy(out[textureHeaderSize:], t.Pixels)
	return out
}

func DecodeTexture(data []byte) (*TextureArtifact, error) {
	if len(data) < textureHeaderSize || string(data[0:4]) != magicTexture {
		return nil, errs.New(errs.AssetBadArtifact, "not a texture artifact")
	}
	version := binary.LittleEndian.Uint32(data[4:8])
	if version != 1 && version != 2 && version != textureVersion {
		return nil, errs.New(errs.AssetBadArtifact, "unsupported texture artifact version",
			log.F("version", version))
	}
	t := &TextureArtifact{
		Width:  int(binary.LittleEndian.Uint32(data[8:12])),
		Height: int(binary.LittleEndian.Uint32(data[12:16])),
		Format: gpu.TextureFormat(data[16]),
		Filter: gpu.FilterMode(data[17]),
		Mips:   data[18] != 0,
		Pixels: data[textureHeaderSize:],
	}
	if version >= 2 {
		t.Semantic = TextureSemantic(data[19])
	}
	if (version < 3 && t.Semantic > TextureHeight) || t.Semantic > TextureCookieMask || (t.Semantic != TextureColor && t.Semantic != TextureEmissionRGB && t.Format != gpu.TexRGBA8) || (t.Format != gpu.TexRGBA8 && t.Format != gpu.TexSRGBA8) {
		return nil, errs.New(errs.AssetBadArtifact, "texture artifact has incompatible format and semantic")
	}
	// Division bounds the product on 32-bit wasm as well as desktop builds.
	if t.Width <= 0 || t.Height <= 0 || t.Width > len(t.Pixels)/4 || t.Height > len(t.Pixels)/4/t.Width {
		return nil, errs.New(errs.AssetBadArtifact, "texture artifact is truncated",
			log.F("w", t.Width), log.F("h", t.Height), log.F("bytes", len(t.Pixels)))
	}
	return t, nil
}

type RegionArtifact struct {
	Pivot    [2]float32
	HasPivot bool
	// Source size and trim offset are zero for legacy/untrimmed regions.
	SourceW, SourceH, TrimX, TrimY int
	Page                           ids.AssetID
	// Normal is a stable reference to an independently packed normal region.
	// A missing referenced asset is retained for later repair; zero means flat.
	Normal       ids.AssetID
	Emission     ids.AssetID
	X, Y, W, H   int
	PageW, PageH int
	Filter       gpu.FilterMode
}

func EncodeRegion(r *RegionArtifact) []byte {
	size, version := regionSize, uint32(regionVersion)
	if r.HasPivot {
		size += 8
		version = 5
	}
	out := make([]byte, size)
	copy(out[0:4], magicRegion)
	binary.LittleEndian.PutUint32(out[4:8], version)
	binary.LittleEndian.PutUint64(out[8:16], r.Page.Hi)
	binary.LittleEndian.PutUint64(out[16:24], r.Page.Lo)
	binary.LittleEndian.PutUint32(out[24:28], uint32(r.X))
	binary.LittleEndian.PutUint32(out[28:32], uint32(r.Y))
	binary.LittleEndian.PutUint32(out[32:36], uint32(r.W))
	binary.LittleEndian.PutUint32(out[36:40], uint32(r.H))
	binary.LittleEndian.PutUint16(out[40:42], uint16(r.PageW))
	binary.LittleEndian.PutUint16(out[42:44], uint16(r.PageH))
	out[44] = byte(r.Filter)
	binary.LittleEndian.PutUint16(out[48:50], uint16(r.SourceW))
	binary.LittleEndian.PutUint16(out[50:52], uint16(r.SourceH))
	binary.LittleEndian.PutUint16(out[52:54], uint16(r.TrimX))
	binary.LittleEndian.PutUint16(out[54:56], uint16(r.TrimY))
	binary.LittleEndian.PutUint64(out[56:64], r.Normal.Hi)
	binary.LittleEndian.PutUint64(out[64:72], r.Normal.Lo)
	binary.LittleEndian.PutUint64(out[72:80], r.Emission.Hi)
	binary.LittleEndian.PutUint64(out[80:88], r.Emission.Lo)
	if r.HasPivot {
		for i, v := range r.Pivot {
			binary.LittleEndian.PutUint32(out[88+i*4:], math.Float32bits(v))
		}
	}
	return out
}

func DecodeRegion(data []byte) (*RegionArtifact, error) {
	if len(data) < 48 || string(data[0:4]) != magicRegion {
		return nil, errs.New(errs.AssetBadArtifact, "not a sprite region artifact")
	}
	version := binary.LittleEndian.Uint32(data[4:8])
	if version != 1 && version != 2 && version != 3 && version != regionVersion && version != 5 {
		return nil, errs.New(errs.AssetBadArtifact, "unsupported region artifact version",
			log.F("version", version))
	}
	if (version >= 2 && len(data) < 56) || (version >= 3 && len(data) < 72) || (version >= 4 && len(data) < regionSize) || (version == 5 && len(data) < regionSize+8) {
		return nil, errs.New(errs.AssetBadArtifact, "sprite region artifact is truncated")
	}
	r := &RegionArtifact{
		Page:   ids.AssetID{Hi: binary.LittleEndian.Uint64(data[8:16]), Lo: binary.LittleEndian.Uint64(data[16:24])},
		X:      int(binary.LittleEndian.Uint32(data[24:28])),
		Y:      int(binary.LittleEndian.Uint32(data[28:32])),
		W:      int(binary.LittleEndian.Uint32(data[32:36])),
		H:      int(binary.LittleEndian.Uint32(data[36:40])),
		PageW:  int(binary.LittleEndian.Uint16(data[40:42])),
		PageH:  int(binary.LittleEndian.Uint16(data[42:44])),
		Filter: gpu.FilterMode(data[44]),
	}
	if version >= 2 {
		r.SourceW = int(binary.LittleEndian.Uint16(data[48:50]))
		r.SourceH = int(binary.LittleEndian.Uint16(data[50:52]))
		r.TrimX = int(binary.LittleEndian.Uint16(data[52:54]))
		r.TrimY = int(binary.LittleEndian.Uint16(data[54:56]))
	}
	if version >= 3 {
		r.Normal = ids.AssetID{Hi: binary.LittleEndian.Uint64(data[56:64]), Lo: binary.LittleEndian.Uint64(data[64:72])}
	}
	if version >= 4 {
		r.Emission = ids.AssetID{Hi: binary.LittleEndian.Uint64(data[72:80]), Lo: binary.LittleEndian.Uint64(data[80:88])}
	}
	if version == 5 {
		r.HasPivot = true
		for i := range r.Pivot {
			v := math.Float32frombits(binary.LittleEndian.Uint32(data[88+i*4:]))
			if math.IsNaN(float64(v)) || v < 0 || v > 1 {
				return nil, errs.New(errs.AssetBadArtifact, "invalid sprite pivot")
			}
			r.Pivot[i] = v
		}
	}
	if r.X < 0 || r.Y < 0 || r.W <= 0 || r.H <= 0 || r.PageW <= 0 || r.PageH <= 0 || r.X > r.PageW || r.Y > r.PageH || r.W > r.PageW-r.X || r.H > r.PageH-r.Y {
		return nil, errs.New(errs.AssetBadArtifact, "region artifact has invalid bounds")
	}
	if (r.SourceW == 0) != (r.SourceH == 0) || (r.SourceW == 0 && (r.TrimX != 0 || r.TrimY != 0)) || (r.SourceW > 0 && (r.TrimX > r.SourceW || r.TrimY > r.SourceH || r.W > r.SourceW-r.TrimX || r.H > r.SourceH-r.TrimY)) {
		return nil, errs.New(errs.AssetBadArtifact, "sprite trim is outside original source bounds")
	}
	return r, nil
}
