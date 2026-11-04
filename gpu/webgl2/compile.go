//go:build js

package webgl2

import (
	"slices"
	"strings"
	"syscall/js"

	"aego/core/errs"
	"aego/core/log"
	"aego/gpu"
)

const glCompletionStatusKHR = 0x91B1

type compilation struct {
	d          *Device
	desc       gpu.ShaderDesc
	program    js.Value
	stages     []js.Value
	generation uint64
	done       bool
	result     gpu.ShaderHandle
	err        error
}

// BeginShaderCompile submits all stages and links without querying compile/link
// status. With KHR_parallel_shader_compile, Poll waits through completion queries.
// Without it Poll performs the documented synchronous fallback at the load/reload
// boundary; no background thread or universal asynchrony is implied.
func (d *Device) BeginShaderCompile(desc gpu.ShaderDesc) (gpu.ShaderCompilation, error) {
	if d.lost || len(desc.Sources) == 0 {
		return nil, errs.New(errs.GPUShaderLink, "cannot start shader compilation without a live context and stages", log.F("label", desc.Label))
	}
	job := &compilation{d: d, desc: gpu.ShaderDesc{Label: desc.Label, Sources: slices.Clone(desc.Sources)}, generation: d.contextGen}
	job.program = d.call(d.f.CreateProgram)
	if !job.program.Truthy() {
		return nil, d.shaderCreationError(errs.GPUShaderLink, "createProgram", desc.Label)
	}
	for _, src := range desc.Sources {
		stage := d.call(d.f.CreateShader, stageType(src.Stage))
		if !stage.Truthy() {
			job.Cancel()
			return nil, d.shaderCreationError(errs.GPUShaderCompile, "createShader", desc.Label)
		}
		job.stages = append(job.stages, stage)
		d.call(d.f.ShaderSource, stage, src.Source)
		d.call(d.f.CompileShader, stage)
		d.call(d.f.AttachShader, job.program, stage)
	}
	d.call(d.f.LinkProgram, job.program)
	d.compiles[job] = struct{}{}
	return job, nil
}

func (j *compilation) cleanup() {
	d := j.d
	if !d.lost && j.generation == d.contextGen {
		for _, stage := range j.stages {
			d.call(d.f.DetachShader, j.program, stage)
			d.call(d.f.DeleteShader, stage)
		}
	}
	j.stages = nil
	delete(d.compiles, j)
}

func (j *compilation) Cancel() {
	if j.done {
		return
	}
	j.cleanup()
	if !j.d.lost && j.generation == j.d.contextGen && j.program.Truthy() {
		j.d.call(j.d.f.DeleteProgram, j.program)
	}
	j.program = js.Null()
	j.done = true
	j.err = errs.New(errs.GPUShaderLink, "shader candidate cancelled", log.F("label", j.desc.Label))
}

func (j *compilation) Poll() (gpu.ShaderHandle, bool, error) {
	if j.done {
		return j.result, true, j.err
	}
	d := j.d
	if d.lost || j.generation != d.contextGen {
		j.Cancel()
		return gpu.ShaderHandle{}, true, j.err
	}
	if d.parallel.Truthy() && !d.call(d.f.GetProgramParameter, j.program, glCompletionStatusKHR).Truthy() {
		return gpu.ShaderHandle{}, false, nil
	}
	linked := d.call(d.f.GetProgramParameter, j.program, glLinkStatus).Truthy()
	if !linked {
		for i, stage := range j.stages {
			if !d.call(d.f.GetShaderParameter, stage, glCompileStatus).Truthy() {
				info := strings.TrimSpace(d.call(d.f.GetShaderInfoLog, stage).String())
				j.err = errs.New(errs.GPUShaderCompile, "shader compile failed", log.F("label", j.desc.Label), log.F("stage", j.desc.Sources[i].Stage.String()), log.F("log", info))
				break
			}
		}
		if j.err == nil {
			j.err = errs.New(errs.GPUShaderLink, "shader link failed", log.F("label", j.desc.Label), log.F("log", strings.TrimSpace(d.call(d.f.GetProgramInfoLog, j.program).String())))
		}
		j.cleanup()
		d.call(d.f.DeleteProgram, j.program)
		j.program = js.Null()
		j.done = true
		return gpu.ShaderHandle{}, true, j.err
	}
	j.cleanup()
	id := d.obj.put(j.program)
	d.bindProgramSlots(j.program, id, j.desc.Label)
	h, s := d.shaders.Alloc()
	*s = glShader{program: id, desc: j.desc}
	j.result, j.done = gpu.ShaderHandle(h), true
	return j.result, true, nil
}
