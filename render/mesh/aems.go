package mesh

import (
	"encoding/binary"
	"math"

	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
)

const (
	aemsMagic   = "AEMS"
	AEMSVersion = 1
	aemsHeader  = 40
)

func putF32(b []byte, v float32) {
	binary.LittleEndian.PutUint32(b, math.Float32bits(v))
}

func getF32(b []byte) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(b))
}

func putVec3(b []byte, v mathx.Vec3) {
	putF32(b[0:], v.X)
	putF32(b[4:], v.Y)
	putF32(b[8:], v.Z)
}

func getVec3(b []byte) mathx.Vec3 {
	return mathx.Vec3{X: getF32(b[0:]), Y: getF32(b[4:]), Z: getF32(b[8:])}
}

func Encode(d *Data) []byte {
	out := make([]byte, aemsHeader+len(d.Vertices)*VertexStride+len(d.Indices)*4)
	copy(out[0:4], aemsMagic)
	binary.LittleEndian.PutUint32(out[4:], AEMSVersion)
	binary.LittleEndian.PutUint32(out[8:], uint32(len(d.Vertices)))
	binary.LittleEndian.PutUint32(out[12:], uint32(len(d.Indices)))
	putVec3(out[16:], d.Bounds.Min)
	putVec3(out[28:], d.Bounds.Max)
	off := aemsHeader
	for _, v := range d.Vertices {
		putVec3(out[off:], v.Pos)
		putVec3(out[off+12:], v.Normal)
		putF32(out[off+24:], v.UV.X)
		putF32(out[off+28:], v.UV.Y)
		binary.LittleEndian.PutUint32(out[off+32:], v.Color)
		off += VertexStride
	}
	for _, i := range d.Indices {
		binary.LittleEndian.PutUint32(out[off:], i)
		off += 4
	}
	return out
}

func Decode(b []byte) (*Data, error) {
	if len(b) < aemsHeader || string(b[0:4]) != aemsMagic {
		return nil, errs.New(errs.AssetBadArtifact, "not a mesh artifact")
	}
	if v := binary.LittleEndian.Uint32(b[4:]); v != AEMSVersion {
		return nil, errs.New(errs.AssetBadArtifact, "unsupported mesh artifact version", log.F("version", v))
	}
	vc := int(binary.LittleEndian.Uint32(b[8:]))
	ic := int(binary.LittleEndian.Uint32(b[12:]))
	if vc < 0 || ic < 0 || len(b) != aemsHeader+vc*VertexStride+ic*4 {
		return nil, errs.New(errs.AssetBadArtifact, "mesh artifact is truncated",
			log.F("vertices", vc), log.F("indices", ic), log.F("bytes", len(b)))
	}
	d := &Data{
		Vertices: make([]Vertex, vc),
		Indices:  make([]uint32, ic),
		Bounds:   mathx.AABB3{Min: getVec3(b[16:]), Max: getVec3(b[28:])},
	}
	off := aemsHeader
	for i := range d.Vertices {
		d.Vertices[i] = Vertex{
			Pos:    getVec3(b[off:]),
			Normal: getVec3(b[off+12:]),
			UV:     mathx.Vec2{X: getF32(b[off+24:]), Y: getF32(b[off+28:])},
			Color:  binary.LittleEndian.Uint32(b[off+32:]),
		}
		off += VertexStride
	}
	for i := range d.Indices {
		d.Indices[i] = binary.LittleEndian.Uint32(b[off:])
		off += 4
	}
	if err := d.Validate(); err != nil {
		return nil, errs.Wrap(err, errs.AssetBadArtifact, "mesh artifact is invalid")
	}
	return d, nil
}
