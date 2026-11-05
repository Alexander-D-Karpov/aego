package asset

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"aego/core/ids"
	"aego/gpu"
)

// TryArtifacts is a bounded prepared payload for documentation sessions, not a game archive; exports still use their directory manifest.
// Items contain the asset loader's runtime artifacts; SourceHash covers the source/meta manifest and importer versions at build time.
type TryArtifacts struct {
	SourceHash [32]byte
	Assets     []TryAsset
}

type TryAsset struct {
	ID        ids.AssetID
	Kind      ArtifactKind
	Name      string
	Deps      []ids.AssetID
	Artifacts []TryArtifact
}

type TryArtifact struct {
	Kind ArtifactKind
	Name string
	Data []byte
}

const (
	TryArtifactsVersion  = 1
	MaxTryAssets         = 64
	MaxTryArtifactBytes  = 2 << 20
	MaxTryArtifactsBytes = 8 << 20
	tryArtifactsHeader   = 76
)

var tryKinds = [...]ArtifactKind{"", ArtTexture, ArtAtlasPage, ArtSpriteRegion, ArtMaterial, ArtShader}

func tryKindCode(kind ArtifactKind) byte {
	for i, candidate := range tryKinds {
		if candidate == kind {
			return byte(i)
		}
	}
	return 0
}

func compareTryID(a, b ids.AssetID) int {
	if a.Hi < b.Hi {
		return -1
	}
	if a.Hi > b.Hi {
		return 1
	}
	if a.Lo < b.Lo {
		return -1
	}
	if a.Lo > b.Lo {
		return 1
	}
	return 0
}

func validTryName(name string) bool {
	return len(name) <= 256 && utf8.ValidString(name) && !strings.ContainsAny(name, "\x00\r\n")
}

func validateTryArtifacts(bundle *TryArtifacts) error {
	if bundle == nil || bundle.SourceHash == ([32]byte{}) || len(bundle.Assets) == 0 || len(bundle.Assets) > MaxTryAssets {
		return fmt.Errorf("try artifacts need a source hash and 1..%d assets", MaxTryAssets)
	}
	byID := make(map[ids.AssetID]*TryAsset, len(bundle.Assets))
	bytes := 0
	for i := range bundle.Assets {
		entry := &bundle.Assets[i]
		if entry.ID.IsZero() || IsBuiltin(entry.ID) || byID[entry.ID] != nil || tryKindCode(entry.Kind) == 0 || !validTryName(entry.Name) || len(entry.Deps) > MaxTryAssets || len(entry.Artifacts) == 0 || len(entry.Artifacts) > 4 {
			return fmt.Errorf("invalid or duplicate try asset %s", entry.ID)
		}
		byID[entry.ID] = entry
		seen := make(map[ArtifactKind]bool, len(entry.Artifacts))
		for _, artifact := range entry.Artifacts {
			allowed := artifact.Kind == entry.Kind || (entry.Kind == ArtTexture || entry.Kind == ArtAtlasPage) && (artifact.Kind == ArtTexture || artifact.Kind == ArtSpriteRegion)
			if !allowed || seen[artifact.Kind] || !validTryName(artifact.Name) || len(artifact.Data) == 0 || len(artifact.Data) > MaxTryArtifactBytes {
				return fmt.Errorf("invalid artifact in try asset %s", entry.ID)
			}
			seen[artifact.Kind] = true
			bytes += len(artifact.Data)
			if bytes > MaxTryArtifactsBytes {
				return fmt.Errorf("try artifact payload exceeds %d bytes", MaxTryArtifactsBytes)
			}
		}
		main := entry.Kind
		if main == ArtAtlasPage {
			main = ArtTexture
		}
		if !seen[main] && !(entry.Kind == ArtAtlasPage && seen[ArtAtlasPage]) {
			return fmt.Errorf("try asset %s has no %s artifact", entry.ID, main)
		}
	}
	for i := range bundle.Assets {
		entry := &bundle.Assets[i]
		seen := map[ids.AssetID]bool{}
		for _, dependency := range entry.Deps {
			_, builtin := LookupBuiltin(dependency)
			if dependency.IsZero() || dependency == entry.ID || seen[dependency] || byID[dependency] == nil && !builtin {
				return fmt.Errorf("try asset %s has a missing, duplicate or self dependency %s", entry.ID, dependency)
			}
			seen[dependency] = true
		}
		require := func(id ids.AssetID) error {
			if id == entry.ID || id.IsZero() {
				return nil
			}
			if !seen[id] {
				return fmt.Errorf("try asset %s omits artifact dependency %s", entry.ID, id)
			}
			return nil
		}
		for _, artifact := range entry.Artifacts {
			var err error
			switch artifact.Kind {
			case ArtTexture, ArtAtlasPage:
				var texture *TextureArtifact
				texture, err = DecodeTexture(artifact.Data)
				if err == nil && (texture.Width > 2048 || texture.Height > 2048 || texture.Filter > gpu.FilterLinear || len(texture.Pixels) != texture.Width*texture.Height*4) {
					err = fmt.Errorf("try texture dimensions, filter or pixel size are invalid")
				}
			case ArtSpriteRegion:
				var region *RegionArtifact
				region, err = DecodeRegion(artifact.Data)
				if err == nil {
					err = require(region.Page)
				}
				if err == nil {
					err = require(region.Normal)
				}
			case ArtMaterial:
				var document *MaterialDoc
				document, err = DecodeMaterial(artifact.Data, entry.Name)
				if err == nil {
					for _, id := range document.Deps() {
						if err = require(id); err != nil {
							break
						}
					}
				}
			case ArtShader:
				_, err = DecodeShader(artifact.Data)
			default:
				err = fmt.Errorf("unsupported try artifact kind %s", artifact.Kind)
			}
			if err != nil {
				return fmt.Errorf("try asset %s: %w", entry.ID, err)
			}
		}
	}
	// Cycles are diagnosed before anything reaches a loader or GPU resource.
	state := make(map[ids.AssetID]uint8, len(byID))
	var visit func(ids.AssetID) error
	visit = func(id ids.AssetID) error {
		if state[id] == 1 {
			return fmt.Errorf("try artifact dependency cycle at %s", id)
		}
		if state[id] == 2 || byID[id] == nil {
			return nil
		}
		state[id] = 1
		for _, dep := range byID[id].Deps {
			if err := visit(dep); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for _, entry := range bundle.Assets {
		if err := visit(entry.ID); err != nil {
			return err
		}
	}
	return nil
}

// EncodeTryArtifacts canonicalizes entry/dependency order and checks every
// prepared decoder. Hashes cover source provenance, descriptors and all bytes.
func EncodeTryArtifacts(bundle *TryArtifacts) ([]byte, error) {
	if err := validateTryArtifacts(bundle); err != nil {
		return nil, err
	}
	entries := slices.Clone(bundle.Assets)
	slices.SortFunc(entries, func(a, b TryAsset) int { return compareTryID(a.ID, b.ID) })
	out := make([]byte, tryArtifactsHeader)
	copy(out, "AETB")
	binary.LittleEndian.PutUint32(out[4:8], TryArtifactsVersion)
	binary.LittleEndian.PutUint32(out[8:12], uint32(len(entries)))
	copy(out[12:44], bundle.SourceHash[:])
	appendID := func(id ids.AssetID) {
		out = binary.LittleEndian.AppendUint64(out, id.Hi)
		out = binary.LittleEndian.AppendUint64(out, id.Lo)
	}
	appendString := func(text string) {
		out = binary.LittleEndian.AppendUint16(out, uint16(len(text)))
		out = append(out, text...)
	}
	for _, entry := range entries {
		appendID(entry.ID)
		out = append(out, tryKindCode(entry.Kind))
		appendString(entry.Name)
		deps := slices.Clone(entry.Deps)
		slices.SortFunc(deps, compareTryID)
		out = binary.LittleEndian.AppendUint16(out, uint16(len(deps)))
		for _, dep := range deps {
			appendID(dep)
		}
		artifacts := slices.Clone(entry.Artifacts)
		slices.SortFunc(artifacts, func(a, b TryArtifact) int { return strings.Compare(string(a.Kind), string(b.Kind)) })
		out = append(out, byte(len(artifacts)))
		for _, artifact := range artifacts {
			out = append(out, tryKindCode(artifact.Kind))
			appendString(artifact.Name)
			out = binary.LittleEndian.AppendUint32(out, uint32(len(artifact.Data)))
			hash := sha256.Sum256(artifact.Data)
			out = append(out, hash[:]...)
			out = append(out, artifact.Data...)
		}
	}
	if len(out) > MaxTryArtifactsBytes {
		return nil, fmt.Errorf("try artifact manifest exceeds %d bytes", MaxTryArtifactsBytes)
	}
	hash := tryArtifactsHash(out)
	copy(out[44:76], hash[:])
	return out, nil
}

func tryArtifactsHash(data []byte) [32]byte {
	hash := sha256.New()
	hash.Write(data[4:44])
	hash.Write(data[76:])
	var out [32]byte
	copy(out[:], hash.Sum(nil))
	return out
}

type tryArtifactReader struct {
	data []byte
	at   int
	err  error
}

func (r *tryArtifactReader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > len(r.data)-r.at {
		r.err = fmt.Errorf("truncated try artifact manifest")
		return nil
	}
	out := r.data[r.at : r.at+n]
	r.at += n
	return out
}
func (r *tryArtifactReader) u8() byte {
	b := r.take(1)
	if len(b) != 1 {
		return 0
	}
	return b[0]
}
func (r *tryArtifactReader) u16() uint16 {
	b := r.take(2)
	if len(b) != 2 {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}
func (r *tryArtifactReader) u32() uint32 {
	b := r.take(4)
	if len(b) != 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}
func (r *tryArtifactReader) id() ids.AssetID {
	b := r.take(16)
	if len(b) != 16 {
		return ids.AssetID{}
	}
	return ids.AssetID{Hi: binary.LittleEndian.Uint64(b), Lo: binary.LittleEndian.Uint64(b[8:])}
}
func (r *tryArtifactReader) text() string {
	n := int(r.u16())
	if n > 256 {
		r.err = fmt.Errorf("try artifact name exceeds 256 bytes")
		return ""
	}
	return string(r.take(n))
}
func (r *tryArtifactReader) kind() ArtifactKind {
	b := r.u8()
	if b == 0 || int(b) >= len(tryKinds) {
		r.err = fmt.Errorf("unknown try artifact kind")
		return ""
	}
	return tryKinds[b]
}

// DecodeTryArtifacts borrows immutable artifact bytes from data. It checks
// bounds before allocating, verifies all hashes and validates dependency closure.
func DecodeTryArtifacts(data []byte) (*TryArtifacts, error) {
	if len(data) < tryArtifactsHeader || len(data) > MaxTryArtifactsBytes || string(data[:4]) != "AETB" || binary.LittleEndian.Uint32(data[4:8]) != TryArtifactsVersion {
		return nil, fmt.Errorf("unsupported or oversized try artifact manifest")
	}
	count := binary.LittleEndian.Uint32(data[8:12])
	if count == 0 || count > MaxTryAssets {
		return nil, fmt.Errorf("try artifact asset count is outside budget")
	}
	hash := tryArtifactsHash(data)
	if !bytes.Equal(hash[:], data[44:76]) {
		return nil, fmt.Errorf("try artifact manifest hash mismatch")
	}
	bundle := &TryArtifacts{Assets: make([]TryAsset, int(count))}
	copy(bundle.SourceHash[:], data[12:44])
	r := tryArtifactReader{data: data, at: tryArtifactsHeader}
	for i := range bundle.Assets {
		entry := &bundle.Assets[i]
		entry.ID, entry.Kind, entry.Name = r.id(), r.kind(), r.text()
		deps := int(r.u16())
		if deps > MaxTryAssets {
			return nil, fmt.Errorf("try artifact dependency budget exceeded")
		}
		entry.Deps = make([]ids.AssetID, deps)
		for j := range entry.Deps {
			entry.Deps[j] = r.id()
		}
		artifacts := int(r.u8())
		if artifacts == 0 || artifacts > 4 {
			return nil, fmt.Errorf("try artifact per-asset budget exceeded")
		}
		entry.Artifacts = make([]TryArtifact, artifacts)
		for j := range entry.Artifacts {
			artifact := &entry.Artifacts[j]
			artifact.Kind, artifact.Name = r.kind(), r.text()
			size := r.u32()
			if size > MaxTryArtifactBytes {
				return nil, fmt.Errorf("try artifact byte budget exceeded")
			}
			want := r.take(32)
			artifact.Data = r.take(int(size))
			if r.err != nil {
				return nil, r.err
			}
			got := sha256.Sum256(artifact.Data)
			if !bytes.Equal(got[:], want) {
				return nil, fmt.Errorf("try artifact %s hash mismatch", entry.ID)
			}
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	if r.at != len(data) {
		return nil, fmt.Errorf("trailing try artifact data")
	}
	if err := validateTryArtifacts(bundle); err != nil {
		return nil, err
	}
	return bundle, nil
}
