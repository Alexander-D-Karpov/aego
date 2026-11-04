package gl33

import (
	"math"
	"unsafe"

	"aego/gpu"
	"aego/gpu/gl33/gl"
)

// probeRGBA16F checks framebuffer completeness, values above one, blending, and
// float readback on this context. A format enum alone is not an HDR capability.
func (d *Device) probeRGBA16F() bool {
	oldTexture := uint32(getInt(gl.TEXTURE_BINDING_2D))
	oldDraw, oldRead := uint32(getInt(gl.DRAW_FRAMEBUFFER_BINDING)), uint32(getInt(gl.READ_FRAMEBUFFER_BINDING))
	oldProgram, oldVAO := uint32(getInt(gl.CURRENT_PROGRAM)), uint32(getInt(gl.VERTEX_ARRAY_BINDING))
	var viewport [4]int32
	var clearColor [4]float32
	var colorMask [4]bool
	gl.GetIntegerv(gl.VIEWPORT, &viewport[0])
	gl.GetFloatv(gl.COLOR_CLEAR_VALUE, &clearColor[0])
	gl.GetBooleanv(gl.COLOR_WRITEMASK, &colorMask[0])
	blend := [6]uint32{uint32(getInt(gl.BLEND_SRC_RGB)), uint32(getInt(gl.BLEND_DST_RGB)), uint32(getInt(gl.BLEND_SRC_ALPHA)), uint32(getInt(gl.BLEND_DST_ALPHA)), uint32(getInt(gl.BLEND_EQUATION_RGB)), uint32(getInt(gl.BLEND_EQUATION_ALPHA))}
	states := [4]uint32{gl.BLEND, gl.DEPTH_TEST, gl.CULL_FACE, gl.SCISSOR_TEST}
	var enabled [4]bool
	for i, state := range states {
		enabled[i] = gl.IsEnabled(state)
	}
	var tex, fbo, vao, prog uint32
	defer func() {
		gl.BindFramebuffer(gl.DRAW_FRAMEBUFFER, oldDraw)
		gl.BindFramebuffer(gl.READ_FRAMEBUFFER, oldRead)
		gl.UseProgram(oldProgram)
		gl.BindVertexArray(oldVAO)
		gl.BindTexture(gl.TEXTURE_2D, oldTexture)
		gl.Viewport(viewport[0], viewport[1], viewport[2], viewport[3])
		gl.ClearColor(clearColor[0], clearColor[1], clearColor[2], clearColor[3])
		gl.ColorMask(colorMask[0], colorMask[1], colorMask[2], colorMask[3])
		gl.BlendFuncSeparate(blend[0], blend[1], blend[2], blend[3])
		gl.BlendEquationSeparate(blend[4], blend[5])
		for i, state := range states {
			if enabled[i] {
				gl.Enable(state)
			} else {
				gl.Disable(state)
			}
		}
		if prog != 0 {
			gl.DeleteProgram(prog)
		}
		gl.DeleteVertexArrays(1, &vao)
		gl.DeleteFramebuffers(1, &fbo)
		gl.DeleteTextures(1, &tex)
		d.cache.Invalidate()
	}()
	for i := 0; i < 16 && gl.GetError() != gl.NO_ERROR; i++ {
	}
	gl.GenTextures(1, &tex)
	gl.BindTexture(gl.TEXTURE_2D, tex)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA16F, 1, 1, 0, gl.RGBA, gl.FLOAT, nil)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.GenFramebuffers(1, &fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, fbo)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, tex, 0)
	gl.DrawBuffer(gl.COLOR_ATTACHMENT0)
	gl.ReadBuffer(gl.COLOR_ATTACHMENT0)
	if gl.CheckFramebufferStatus(gl.FRAMEBUFFER) != gl.FRAMEBUFFER_COMPLETE || gl.GetError() != gl.NO_ERROR {
		return false
	}
	vs, err := compileStage(gpu.ShaderSource{Stage: gpu.StageVertex, Source: `#version 330 core
void main() { vec2 p = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2); gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0); }`})
	if err != nil {
		return false
	}
	defer gl.DeleteShader(vs)
	fs, err := compileStage(gpu.ShaderSource{Stage: gpu.StageFragment, Source: `#version 330 core
out vec4 color;
void main() { color = vec4(2.0, 0.5, 0.25, 0.5); }`})
	if err != nil {
		return false
	}
	defer gl.DeleteShader(fs)
	prog = gl.CreateProgram()
	gl.AttachShader(prog, vs)
	gl.AttachShader(prog, fs)
	gl.LinkProgram(prog)
	var linked int32
	gl.GetProgramiv(prog, gl.LINK_STATUS, &linked)
	if linked == gl.FALSE {
		return false
	}
	gl.GenVertexArrays(1, &vao)
	gl.BindVertexArray(vao)
	gl.UseProgram(prog)
	gl.Disable(gl.DEPTH_TEST)
	gl.Disable(gl.CULL_FACE)
	gl.Disable(gl.SCISSOR_TEST)
	gl.ColorMask(true, true, true, true)
	gl.Viewport(0, 0, 1, 1)
	gl.ClearColor(1, 0, 0, 0.25)
	gl.Clear(gl.COLOR_BUFFER_BIT)
	gl.Enable(gl.BLEND)
	gl.BlendEquationSeparate(gl.FUNC_ADD, gl.FUNC_ADD)
	gl.BlendFuncSeparate(gl.ONE, gl.ONE, gl.ONE, gl.ONE)
	gl.DrawArrays(gl.TRIANGLES, 0, 3)
	var pixel [4]float32
	gl.ReadPixels(0, 0, 1, 1, gl.RGBA, gl.FLOAT, unsafe.Pointer(&pixel[0]))
	if gl.GetError() != gl.NO_ERROR {
		return false
	}
	for i, want := range [4]float32{3, 0.5, 0.25, 0.75} {
		if math.IsNaN(float64(pixel[i])) || math.Abs(float64(pixel[i]-want)) > 0.01 {
			return false
		}
	}
	return true
}
