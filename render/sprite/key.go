package sprite

import "aego/core/mathx"

type SortKey uint64

const (
	layerBits  = 12
	orderBits  = 12
	zBits      = 16
	submitBits = 24

	LayerMin int16 = -2048
	LayerMax int16 = 2047
	OrderMin int16 = -2048
	OrderMax int16 = 2047

	MaxSubmission = 1<<submitBits - 1
)

func MakeKey(layer, order int16, z, zMin, zMax float32, submission uint32) SortKey {
	l := uint64(clampAxis(layer)) << (orderBits + zBits + submitBits)
	o := uint64(clampAxis(order)) << (zBits + submitBits)
	q := uint64(quantizeZ(z, zMin, zMax)) << submitBits
	return SortKey(l | o | q | uint64(submission&MaxSubmission))
}

func (k SortKey) Submission() uint32 {
	return uint32(k) & MaxSubmission
}

func clampAxis(v int16) uint16 {
	if v < LayerMin {
		v = LayerMin
	}
	if v > LayerMax {
		v = LayerMax
	}
	return uint16(int32(v) + 2048)
}

func quantizeZ(z, zMin, zMax float32) uint16 {
	if zMax <= zMin {
		return 0
	}
	t := mathx.Clamp((z-zMin)/(zMax-zMin), 0, 1)
	return uint16(t * float32(1<<zBits-1))
}

func OutOfRange(layer, order int16, z, zMin, zMax float32) bool {
	return layer < LayerMin || layer > LayerMax ||
		order < OrderMin || order > OrderMax ||
		z < zMin || z > zMax
}
