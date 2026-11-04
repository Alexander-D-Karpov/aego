package shader

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
)

// ValidationFlags records checks actually performed. Reference validation never
// implies that a driver linked the shader on either graphics backend.
type ValidationFlags uint8

const (
	SchemaChecked ValidationFlags = 1 << iota
	ReferenceChecked
	GL33Linked
	WebGL2Linked
)

const AllValidationFlags = SchemaChecked | ReferenceChecked | GL33Linked | WebGL2Linked

// Dependency is an exact source-content dependency, including engine includes
// which do not have a project AssetID.
type Dependency struct {
	Path string
	Hash [32]byte
}

type Descriptor struct {
	Features     Features
	Domain       Domain
	Blend        Blend
	Keywords     []string
	Combos       []uint32
	Params       []Param
	Size         int
	Lanes        int
	Vertex       bool
	Files        []string
	Dependencies []Dependency
}

type Features uint32

const (
	FeatureLighting Features = 1 << iota
	FeatureAffine
	FeatureSurfaceChannels
	FeaturePostSceneLinear
	FeaturePixelCoverage
)

const AllFeatures = FeatureLighting | FeatureAffine | FeatureSurfaceChannels | FeaturePostSceneLinear | FeaturePixelCoverage

// Source is implemented by the dev generator. Runtime assets accept this
// interface without importing the generator or compiler packages.
type Source interface {
	Descriptor() Descriptor
	Source(fragment, es bool, mask uint32) (string, []Loc)
}

func hashUint(h hash.Hash, v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	h.Write(b[:])
}

func hashString(h hash.Hash, s string) {
	hashUint(h, uint32(len(s)))
	h.Write([]byte(s))
}

// LayoutHash includes full-width offsets and the shader ABI. Default values and
// UI hints do not change storage layout.
func LayoutHash(domain Domain, params []Param, size, lanes int) [32]byte {
	h := sha256.New()
	hashUint(h, ABI)
	hashUint(h, uint32(domain))
	hashUint(h, uint32(size))
	hashUint(h, uint32(lanes))
	hashUint(h, uint32(len(params)))
	for _, p := range params {
		hashString(h, p.Name)
		hashUint(h, uint32(p.Kind))
		hashUint(h, uint32(p.Offset))
		hashUint(h, uint32(p.Lane))
		hashUint(h, uint32(p.Comp))
		hashUint(h, uint32(p.Slot))
		var instance uint32
		if p.Instance {
			instance = 1
		}
		hashUint(h, instance)
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

// FeatureLayoutHash preserves legacy AESH2 hashes when no extension is present.
// Prepared AESH3 programs include optional engine resources in their ABI identity.
func FeatureLayoutHash(domain Domain, params []Param, size, lanes int, features Features) [32]byte {
	base := LayoutHash(domain, params, size, lanes)
	if features == 0 {
		return base
	}
	var data [36]byte
	copy(data[:32], base[:])
	binary.LittleEndian.PutUint32(data[32:], uint32(features))
	return sha256.Sum256(data[:])
}
