package material

import (
	"aego/core/ids"
	"aego/render/shader"
)

type Variant struct {
	Mask    uint32
	Sources [4]string
}

func SourceIndex(frag, es bool) int {
	i := 0
	if frag {
		i = 1
	}
	if es {
		i += 2
	}
	return i
}

type Shader struct {
	lightParent            *Shader
	OpaqueWithOpaqueAlbedo bool
	// StockLit identifies the exact built-in forward Lit shader, including its energy contract.
	StockLit bool
	Features shader.Features
	ID       ids.AssetID
	Name     string
	Domain   shader.Domain
	Vertex   bool
	Blend    shader.Blend
	Keywords []string
	Variants []Variant
	Schema   *Schema
	Files    []string
	Maps     [4][]shader.Loc
}

func (s *Shader) Variant(mask uint32) int {
	for i := range s.Variants {
		if s.Variants[i].Mask == mask {
			return i
		}
	}
	return -1
}
