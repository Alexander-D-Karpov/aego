//go:build js

package webgl2

import (
	"slices"
	"strings"
	"syscall/js"

	"aego/core/errs"
	"aego/core/log"
	"aego/gpu"
	"aego/gpu/internal/glstate"
)

func (d *Device) link(desc gpu.ShaderDesc) (js.Value, *errs.Error) {
	if d.lost {
		return js.Null(), errs.New(errs.GPUShaderLink, "cannot link a shader while the WebGL context is lost")
	}
	prog := d.call(d.f.CreateProgram)
	if !prog.Truthy() {
		return js.Null(), d.shaderCreationError(errs.GPUShaderLink, "createProgram", desc.Label)
	}
	stages := make([]js.Value, 0, len(desc.Sources))
	cleanup := func() {
		for _, sh := range stages {
			d.call(d.f.DetachShader, prog, sh)
			d.call(d.f.DeleteShader, sh)
		}
	}
	for _, src := range desc.Sources {
		sh := d.call(d.f.CreateShader, stageType(src.Stage))
		if !sh.Truthy() {
			cleanup()
			d.call(d.f.DeleteProgram, prog)
			return js.Null(), d.shaderCreationError(errs.GPUShaderCompile, "createShader", desc.Label)
		}
		d.call(d.f.ShaderSource, sh, src.Source)
		d.call(d.f.CompileShader, sh)
		if !d.call(d.f.GetShaderParameter, sh, glCompileStatus).Truthy() {
			info := strings.TrimSpace(d.call(d.f.GetShaderInfoLog, sh).String())
			d.call(d.f.DeleteShader, sh)
			cleanup()
			d.call(d.f.DeleteProgram, prog)
			return js.Null(), errs.New(errs.GPUShaderCompile, "shader compile failed",
				log.F("label", desc.Label), log.F("stage", src.Stage.String()), log.F("log", info))
		}
		d.call(d.f.AttachShader, prog, sh)
		stages = append(stages, sh)
	}
	d.call(d.f.LinkProgram, prog)
	ok := d.call(d.f.GetProgramParameter, prog, glLinkStatus).Truthy()
	cleanup()
	if !ok {
		info := strings.TrimSpace(d.call(d.f.GetProgramInfoLog, prog).String())
		d.call(d.f.DeleteProgram, prog)
		return js.Null(), errs.New(errs.GPUShaderLink, "shader link failed", log.F("label", desc.Label), log.F("log", info))
	}
	return prog, nil
}

// A driver can return null before the browser delivers webglcontextlost.
// Query only on creation failure so steady-state rendering adds no GL calls.
func (d *Device) shaderCreationError(code errs.Code, op, label string) *errs.Error {
	if d.call(d.f.IsContextLost).Truthy() {
		d.markLost()
		return errs.New(code, "WebGL context lost during shader creation", log.F("label", label))
	}
	return errs.New(code, op+" returned null", log.F("label", label))
}

func (d *Device) CreateShader(desc gpu.ShaderDesc) (gpu.ShaderHandle, error) {
	if len(desc.Sources) == 0 {
		return gpu.ShaderHandle{}, errs.New(errs.GPUShaderLink, "shader has no stages", log.F("label", desc.Label))
	}
	prog, err := d.link(desc)
	if err != nil {
		return gpu.ShaderHandle{}, err
	}
	id := d.obj.put(prog)
	d.bindProgramSlots(prog, id, desc.Label)
	h, s := d.shaders.Alloc()
	*s = glShader{program: id, desc: gpu.ShaderDesc{Sources: slices.Clone(desc.Sources), Label: desc.Label}}
	return gpu.ShaderHandle(h), nil
}

func (d *Device) DestroyShader(h gpu.ShaderHandle) {
	d.destroy(kindShader, h.Index, h.Gen)
}

func (d *Device) bindProgramSlots(prog js.Value, id uint32, label string) {
	blocks := num(d.call(d.f.GetProgramParameter, prog, glActiveUniformBlocks))
	for i := 0; i < blocks; i++ {
		block := d.call(d.f.GetActiveUniformBlockName, prog, i).String()
		slot, ok := slotIndex(block, "ub")
		if !ok || slot >= glstate.MaxUniformBuffers {
			d.log.Warn("uniform block does not follow ubN naming", log.F("shader", label), log.F("block", block))
			continue
		}
		d.call(d.f.UniformBlockBinding, prog, i, slot)
	}
	uniforms := num(d.call(d.f.GetProgramParameter, prog, glActiveUniforms))
	d.cache.UseProgram(id)
	for i := 0; i < uniforms; i++ {
		info := d.call(d.f.GetActiveUniform, prog, i)
		if !info.Truthy() || !isSamplerType(uint32(num(info.Get("type")))) {
			continue
		}
		name := strings.TrimSuffix(info.Get("name").String(), "[0]")
		unit, ok := slotIndex(name, "tex")
		if !ok || unit >= scratchUnit {
			d.log.Warn("sampler uniform does not follow texN naming", log.F("shader", label), log.F("uniform", name))
			continue
		}
		d.call(d.f.Uniform1i, d.call(d.f.GetUniformLocation, prog, name), unit)
	}
}
