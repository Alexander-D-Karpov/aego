package gl33

import (
	"strconv"
	"strings"

	"aego/core/errs"
	"aego/core/handle"
	"aego/gpu"
	"aego/gpu/gl33/gl"
)

func (d *Device) UniformBlockLayout(h gpu.ShaderHandle, slot int) (gpu.UniformBlockLayout, bool, error) {
	s, ok := d.shaders.Get(handle.Cast[glShader](h))
	if !ok {
		return gpu.UniformBlockLayout{}, false, errs.New(errs.GPUInvalidHandle, "uniform layout: invalid shader")
	}
	name := append([]byte("ub"+strconv.Itoa(slot)), 0)
	block := gl.GetUniformBlockIndex(s.program, &name[0])
	if block == gl.INVALID_INDEX {
		return gpu.UniformBlockLayout{}, false, nil
	}
	var size, count int32
	gl.GetActiveUniformBlockiv(s.program, block, gl.UNIFORM_BLOCK_DATA_SIZE, &size)
	gl.GetActiveUniformBlockiv(s.program, block, gl.UNIFORM_BLOCK_ACTIVE_UNIFORMS, &count)
	out := gpu.UniformBlockLayout{Size: int(size)}
	if count <= 0 {
		return out, true, nil
	}
	indices := make([]int32, count)
	gl.GetActiveUniformBlockiv(s.program, block, gl.UNIFORM_BLOCK_ACTIVE_UNIFORM_INDICES, &indices[0])
	buf := make([]byte, 256)
	for _, raw := range indices {
		index := uint32(raw)
		var length, n, offset int32
		var kind uint32
		gl.GetActiveUniform(s.program, index, int32(len(buf)), &length, &n, &kind, &buf[0])
		gl.GetActiveUniformsiv(s.program, 1, &index, gl.UNIFORM_OFFSET, &offset)
		out.Members = append(out.Members, gpu.UniformMember{Name: strings.TrimSuffix(string(buf[:length]), "[0]"), Type: uniformType(kind), Offset: int(offset)})
	}
	return out, true, nil
}

func uniformType(t uint32) string {
	switch t {
	case gl.FLOAT:
		return "float"
	case gl.INT:
		return "int"
	case gl.BOOL:
		return "bool"
	case gl.FLOAT_VEC2:
		return "vec2"
	case gl.FLOAT_VEC3:
		return "vec3"
	case gl.FLOAT_VEC4:
		return "vec4"
	case gl.FLOAT_MAT4:
		return "mat4"
	}
	return "unknown"
}
