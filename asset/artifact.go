package asset

import (
	"encoding/binary"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/gpu"
)

const (
	magicTexture = "AETX"
	magicRegion  = "AERG"

	textureVersion = 1
	regionVersion  = 1

	textureHeaderSize = 20
	regionSize        = 48
)

type TextureArtifact struct {
	Width, Height int
	Format        gpu.TextureFormat
	Filter        gpu.FilterMode
	Mips          bool
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
	copy(out[textureHeaderSize:], t.Pixels)
	return out
}

func DecodeTexture(data []byte) (*TextureArtifact, error) {
	if len(data) < textureHeaderSize || string(data[0:4]) != magicTexture {
		return nil, errs.New(errs.AssetBadArtifact, "not a texture artifact")
	}
	if v := binary.LittleEndian.Uint32(data[4:8]); v != textureVersion {
		return nil, errs.New(errs.AssetBadArtifact, "unsupported texture artifact version",
			log.F("version", v))
	}
	t := &TextureArtifact{
		Width:  int(binary.LittleEndian.Uint32(data[8:12])),
		Height: int(binary.LittleEndian.Uint32(data[12:16])),
		Format: gpu.TextureFormat(data[16]),
		Filter: gpu.FilterMode(data[17]),
		Mips:   data[18] != 0,
		Pixels: data[textureHeaderSize:],
	}
	if t.Width <= 0 || t.Height <= 0 || len(t.Pixels) < t.Width*t.Height*4 {
		return nil, errs.New(errs.AssetBadArtifact, "texture artifact is truncated",
			log.F("w", t.Width), log.F("h", t.Height), log.F("bytes", len(t.Pixels)))
	}
	return t, nil
}

type RegionArtifact struct {
	Page         ids.AssetID
	X, Y, W, H   int
	PageW, PageH int
	Filter       gpu.FilterMode
}

func EncodeRegion(r *RegionArtifact) []byte {
	out := make([]byte, regionSize)
	copy(out[0:4], magicRegion)
	binary.LittleEndian.PutUint32(out[4:8], regionVersion)
	binary.LittleEndian.PutUint64(out[8:16], r.Page.Hi)
	binary.LittleEndian.PutUint64(out[16:24], r.Page.Lo)
	binary.LittleEndian.PutUint32(out[24:28], uint32(r.X))
	binary.LittleEndian.PutUint32(out[28:32], uint32(r.Y))
	binary.LittleEndian.PutUint32(out[32:36], uint32(r.W))
	binary.LittleEndian.PutUint32(out[36:40], uint32(r.H))
	binary.LittleEndian.PutUint16(out[40:42], uint16(r.PageW))
	binary.LittleEndian.PutUint16(out[42:44], uint16(r.PageH))
	out[44] = byte(r.Filter)
	return out
}

func DecodeRegion(data []byte) (*RegionArtifact, error) {
	if len(data) < regionSize || string(data[0:4]) != magicRegion {
		return nil, errs.New(errs.AssetBadArtifact, "not a sprite region artifact")
	}
	if v := binary.LittleEndian.Uint32(data[4:8]); v != regionVersion {
		return nil, errs.New(errs.AssetBadArtifact, "unsupported region artifact version",
			log.F("version", v))
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
	if r.W <= 0 || r.H <= 0 || r.PageW <= 0 || r.PageH <= 0 {
		return nil, errs.New(errs.AssetBadArtifact, "region artifact has invalid bounds")
	}
	return r, nil
}
