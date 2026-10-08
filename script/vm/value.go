package vm

import "math"

type Value struct {
	A, B uint64
}

func Int(v int64) Value {
	return Value{A: uint64(v)}
}

func (v Value) Int() int64 {
	return int64(v.A)
}

func Float(f float32) Value {
	return Value{A: uint64(math.Float32bits(f))}
}

func (v Value) Float() float32 {
	return math.Float32frombits(uint32(v.A))
}

func Bool(b bool) Value {
	if b {
		return Value{A: 1}
	}
	return Value{}
}

func (v Value) Bool() bool {
	return v.A != 0
}

func Vec(x, y, z, w float32) Value {
	return Value{
		A: uint64(math.Float32bits(x)) | uint64(math.Float32bits(y))<<32,
		B: uint64(math.Float32bits(z)) | uint64(math.Float32bits(w))<<32,
	}
}

func (v Value) Lane(i int) float32 {
	var bits uint32
	switch i & 3 {
	case 0:
		bits = uint32(v.A)
	case 1:
		bits = uint32(v.A >> 32)
	case 2:
		bits = uint32(v.B)
	default:
		bits = uint32(v.B >> 32)
	}
	return math.Float32frombits(bits)
}

func (v Value) WithLane(i int, f float32) Value {
	b := uint64(math.Float32bits(f))
	switch i & 3 {
	case 0:
		v.A = v.A&^0xFFFFFFFF | b
	case 1:
		v.A = v.A&0xFFFFFFFF | b<<32
	case 2:
		v.B = v.B&^0xFFFFFFFF | b
	default:
		v.B = v.B&0xFFFFFFFF | b<<32
	}
	return v
}

func (v Value) IsZero() bool {
	return v.A == 0 && v.B == 0
}

func (v Value) Len() int {
	return int(v.B)
}

type InstanceID uint32

const NoInstance InstanceID = 0
