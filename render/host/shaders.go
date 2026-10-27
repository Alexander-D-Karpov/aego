package host

import (
	"errors"

	"aego/core/errs"
	"aego/core/log"
	"aego/engine/problems"
	"aego/render/material"
	"aego/render/shaderx"
)

// shaderErrors keeps the driver's shader errors (SHD009) mapped back to the shader's own lines.
type shaderErrors struct {
	diags []problems.Diagnostic
	rev   uint64
}

func errField(err error, key string) string {
	var e *errs.Error
	if !errors.As(err, &e) {
		return ""
	}
	for _, f := range e.Fields {
		if f.Key == key {
			if s, ok := f.Val.(string); ok {
				return s
			}
		}
	}
	return ""
}

func (m *Module) shaderError(sh *material.Shader, _ int, err error) {
	frag := errField(err, "stage") == "fragment"
	diags := shaderx.MapLog(errField(err, "log"), sh.Maps[material.SourceIndex(frag, m.eng.Caps().ESSL)], sh.Files, shaderx.SHD009)
	if len(diags) == 0 {
		diags = []shaderx.Diag{{Code: shaderx.SHD009, File: sh.Name, Msg: err.Error()}}
	}
	lg := m.eng.Log().With(log.Render)
	for _, d := range diags {
		lg.Error("shader does not compile, the previous version stays", log.F("code", d.Code), log.F("file", d.File), log.F("line", d.Line), log.F("err", d.Msg))
		m.shaders.diags = append(m.shaders.diags, problems.Diagnostic{
			Owner: ID, Code: d.Code, Severity: problems.Error, Message: d.Msg,
			Subject: problems.Subject{Kind: problems.SubjectFile, Asset: sh.ID, File: d.File, Line: d.Line},
		})
	}
	m.shaders.rev++
}

// ShaderProblems lists the shaders the driver refused, for the Problems panel.
func (m *Module) ShaderProblems() problems.Source {
	return problems.Source{
		Revision: func() uint64 { return m.shaders.rev },
		Append: func(dst []problems.Diagnostic) []problems.Diagnostic {
			return append(dst, m.shaders.diags...)
		},
	}
}
