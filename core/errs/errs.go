package errs

import (
	"errors"
	"fmt"
	"strings"

	"aego/core/log"
)

type Code string

const (
	PlatformInit      Code = "platform.init"
	PlatformWindow    Code = "platform.window"
	PlatformContext   Code = "platform.context"
	PlatformConfig    Code = "platform.config"
	GPUInit           Code = "gpu.init"
	GPUShaderCompile  Code = "gpu.shader.compile"
	GPUShaderLink     Code = "gpu.shader.link"
	GPUResource       Code = "gpu.resource"
	GPUInvalidHandle  Code = "gpu.handle.invalid"
	VFSNotFound       Code = "vfs.not_found"
	VFSReadOnly       Code = "vfs.read_only"
	VFSBadPath        Code = "vfs.bad_path"
	VFSUnknownScheme  Code = "vfs.unknown_scheme"
	VFSIO             Code = "vfs.io"
	VFSMount          Code = "vfs.mount"
	EngineConfig      Code = "engine.config"
	EngineInit        Code = "engine.init"
	EngineModule      Code = "engine.module"
	EngineFrozen      Code = "engine.registry.frozen"
	EngineSystemDup   Code = "engine.system.duplicate"
	EngineSystemBad   Code = "engine.system.invalid"
	EngineSystemDep   Code = "engine.system.unknown_dependency"
	EngineSystemPhase Code = "engine.system.phase_mismatch"
	EngineSystemCycle Code = "engine.system.cycle"
)

type Error struct {
	Code   Code
	Msg    string
	Cause  error
	Fields []log.Field
}

func New(code Code, msg string, fields ...log.Field) *Error {
	return &Error{Code: code, Msg: msg, Fields: fields}
}

func Wrap(err error, code Code, msg string) *Error {
	return &Error{Code: code, Msg: msg, Cause: err}
}

func (e *Error) With(fields ...log.Field) *Error {
	e.Fields = append(e.Fields, fields...)
	return e
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(string(e.Code))
	if e.Msg != "" {
		b.WriteString(": ")
		b.WriteString(e.Msg)
	}
	for _, f := range e.Fields {
		fmt.Fprintf(&b, " %s=%v", f.Key, f.Val)
	}
	if e.Cause != nil {
		b.WriteString(": ")
		b.WriteString(e.Cause.Error())
	}
	return b.String()
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func CodeOf(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func Has(err error, code Code) bool {
	for err != nil {
		if e, ok := err.(*Error); ok && e.Code == code {
			return true
		}
		err = errors.Unwrap(err)
	}
	return false
}
