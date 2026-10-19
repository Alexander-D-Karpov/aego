package gl33

import (
	"strconv"
	"strings"

	"aego/core/errs"
	"aego/core/log"
	"aego/gpu"
	"aego/gpu/gl33/gl"
	"aego/gpu/internal/glstate"
)

func (d *Device) CreateShader(desc gpu.ShaderDesc) (gpu.ShaderHandle, error) {
	if len(desc.Sources) == 0 {
		return gpu.ShaderHandle{}, errs.New(errs.GPUShaderLink, "shader has no stages", log.F("label", desc.Label))
	}
	prog := gl.CreateProgram()
	stages := make([]uint32, 0, len(desc.Sources))
	cleanup := func() {
		for _, sh := range stages {
			gl.DetachShader(prog, sh)
			gl.DeleteShader(sh)
		}
	}
	for _, src := range desc.Sources {
		sh, err := compileStage(src)
		if err != nil {
			cleanup()
			gl.DeleteProgram(prog)
			return gpu.ShaderHandle{}, err.With(log.F("label", desc.Label))
		}
		gl.AttachShader(prog, sh)
		stages = append(stages, sh)
	}
	gl.LinkProgram(prog)
	var status int32
	gl.GetProgramiv(prog, gl.LINK_STATUS, &status)
	cleanup()
	if status == gl.FALSE {
		info := programLog(prog)
		gl.DeleteProgram(prog)
		return gpu.ShaderHandle{}, errs.New(errs.GPUShaderLink, "shader link failed",
			log.F("label", desc.Label), log.F("log", info))
	}
	d.bindProgramSlots(prog, desc.Label)
	d.label(gl.PROGRAM, prog, desc.Label)
	h, s := d.shaders.Alloc()
	s.program = prog
	return gpu.ShaderHandle(h), nil
}

func (d *Device) DestroyShader(h gpu.ShaderHandle) {
	d.destroy(kindShader, h.Index, h.Gen)
}

func compileStage(src gpu.ShaderSource) (uint32, *errs.Error) {
	sh := gl.CreateShader(stageType(src.Stage))
	cs, free := gl.Strs(src.Source + "\x00")
	gl.ShaderSource(sh, 1, cs, nil)
	free()
	gl.CompileShader(sh)
	var status int32
	gl.GetShaderiv(sh, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		info := shaderLog(sh)
		gl.DeleteShader(sh)
		return 0, errs.New(errs.GPUShaderCompile, "shader compile failed",
			log.F("stage", src.Stage.String()), log.F("log", info))
	}
	return sh, nil
}

func shaderLog(sh uint32) string {
	var n int32
	gl.GetShaderiv(sh, gl.INFO_LOG_LENGTH, &n)
	if n <= 1 {
		return ""
	}
	buf := make([]byte, n)
	gl.GetShaderInfoLog(sh, n, nil, &buf[0])
	return strings.TrimRight(string(buf), "\x00\r\n ")
}

func programLog(prog uint32) string {
	var n int32
	gl.GetProgramiv(prog, gl.INFO_LOG_LENGTH, &n)
	if n <= 1 {
		return ""
	}
	buf := make([]byte, n)
	gl.GetProgramInfoLog(prog, n, nil, &buf[0])
	return strings.TrimRight(string(buf), "\x00\r\n ")
}

func slotIndex(name, prefix string) (int, bool) {
	rest, ok := strings.CutPrefix(name, prefix)
	if !ok || rest == "" {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	return n, err == nil && n >= 0
}

func (d *Device) bindProgramSlots(prog uint32, label string) {
	name := make([]byte, 256)
	var count int32
	gl.GetProgramiv(prog, gl.ACTIVE_UNIFORM_BLOCKS, &count)
	for i := uint32(0); i < uint32(count); i++ {
		var l int32
		gl.GetActiveUniformBlockName(prog, i, int32(len(name)), &l, &name[0])
		block := string(name[:l])
		slot, ok := slotIndex(block, "ub")
		if !ok || slot >= glstate.MaxUniformBuffers {
			d.log.Warn("uniform block does not follow ubN naming", log.F("shader", label), log.F("block", block))
			continue
		}
		gl.UniformBlockBinding(prog, i, uint32(slot))
	}
	gl.GetProgramiv(prog, gl.ACTIVE_UNIFORMS, &count)
	d.cache.UseProgram(prog)
	for i := uint32(0); i < uint32(count); i++ {
		var l, size int32
		var xtype uint32
		gl.GetActiveUniform(prog, i, int32(len(name)), &l, &size, &xtype, &name[0])
		if !isSamplerType(xtype) {
			continue
		}
		uniform := strings.TrimSuffix(string(name[:l]), "[0]")
		unit, ok := slotIndex(uniform, "tex")
		if !ok || unit >= scratchUnit {
			d.log.Warn("sampler uniform does not follow texN naming", log.F("shader", label), log.F("uniform", uniform))
			continue
		}
		cname := append([]byte(uniform), 0)
		gl.Uniform1i(gl.GetUniformLocation(prog, &cname[0]), int32(unit))
	}
}
