//go:build js

package webgl2

import (
	"strconv"
	"strings"
	"syscall/js"

	"aego/core/errs"
	"aego/core/handle"
	"aego/gpu"
)

func (d *Device) UniformBlockLayout(h gpu.ShaderHandle, slot int) (gpu.UniformBlockLayout, bool, error) {
	s, ok := d.shaders.Get(handle.Cast[glShader](h))
	if !ok {
		return gpu.UniformBlockLayout{}, false, errs.New(errs.GPUInvalidHandle, "uniform layout: invalid shader")
	}
	program := d.obj.get(s.program)
	block := d.call(d.f.GetUniformBlockIndex, program, "ub"+strconv.Itoa(slot))
	if block.Type() != js.TypeNumber || block.Float() == 4294967295 {
		return gpu.UniformBlockLayout{}, false, nil
	}
	idx := block.Int()
	out := gpu.UniformBlockLayout{Size: num(d.call(d.f.GetActiveUniformBlockParameter, program, idx, 0x8A40))}
	indices := d.call(d.f.GetActiveUniformBlockParameter, program, idx, 0x8A43)
	offsets := d.call(d.f.GetActiveUniforms, program, indices, 0x8A3B)
	for i := 0; i < indices.Length(); i++ {
		info := d.call(d.f.GetActiveUniform, program, indices.Index(i))
		out.Members = append(out.Members, gpu.UniformMember{Name: strings.TrimSuffix(info.Get("name").String(), "[0]"), Type: uniformType(uint32(num(info.Get("type")))), Offset: num(offsets.Index(i))})
	}
	return out, true, nil
}

func uniformType(t uint32) string {
	switch t {
	case 0x1406:
		return "float"
	case 0x1404:
		return "int"
	case 0x8B56:
		return "bool"
	case 0x8B50:
		return "vec2"
	case 0x8B51:
		return "vec3"
	case 0x8B52:
		return "vec4"
	case 0x8B5C:
		return "mat4"
	}
	return "unknown"
}
