package host

import (
	"errors"
	"fmt"
	"strings"

	"aego/core/errs"
	"aego/core/log"
	"aego/engine/problems"
	"aego/render/material"
	"aego/render/shader"
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

func (m *Module) shaderError(sh *material.Shader, variant int, err error) {
	var mismatch material.DomainMismatch
	if errors.As(err, &mismatch) {
		// A wrong geometry assignment must not evict driver diagnostics or
		// label a successfully linked shader as a rejected reload candidate.
		m.eng.Log().With(log.Render).Error("material cannot be used on this geometry", log.F("code", material.MAT004), log.F("file", sh.Name), log.F("err", mismatch.Error()), log.F("asset", sh.ID.String()))
		m.shaders.diags = append(m.shaders.diags, problems.Diagnostic{Code: material.MAT004, Severity: problems.Error, Owner: "render", Message: mismatch.Error(), Subject: problems.Subject{Kind: problems.SubjectAsset, Asset: sh.ID, File: sh.Name}})
		m.shaders.rev++
		return
	}
	m.shaderReady(sh)
	backend := "GL33"
	if m.eng.Caps().ESSL {
		backend = "WebGL2"
	}
	stage := errField(err, "stage")
	if stage == "" {
		stage = "link/layout"
	}
	var names []string
	if variant >= 0 && variant < len(sh.Variants) {
		for i, k := range sh.Keywords {
			if sh.Variants[variant].Mask&(1<<i) != 0 {
				names = append(names, k)
			}
		}
	}
	variantName := strings.Join(names, "+")
	if variantName == "" {
		variantName = "default"
	}
	frag := errField(err, "stage") == "fragment"
	diags := shader.MapLog(errField(err, "log"), sh.Maps[material.SourceIndex(frag, m.eng.Caps().ESSL)], sh.Files, shader.SHD009)
	if len(diags) == 0 {
		diags = []shader.Diag{{Code: shader.SHD009, File: sh.Name, Msg: err.Error()}}
	}
	lg := m.eng.Log().With(log.Render)
	for _, d := range diags {
		d.Msg = fmt.Sprintf("%s %s variant %s: %s", backend, stage, variantName, d.Msg)
		lg.Error("shader candidate failed, the previous version stays", log.F("code", d.Code), log.F("file", d.File), log.F("line", d.Line), log.F("err", d.Msg), log.F("compiler_log", errField(err, "log")), log.F("asset", sh.ID.String()))
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

func (m *Module) shaderReady(sh *material.Shader) {
	before := len(m.shaders.diags)
	keep := m.shaders.diags[:0]
	for _, d := range m.shaders.diags {
		match := d.Subject.Asset == sh.ID
		if sh.ID.IsZero() {
			match = d.Subject.File == sh.Name
		}
		if !match {
			keep = append(keep, d)
		}
	}
	m.shaders.diags = keep
	if len(keep) != before {
		m.shaders.rev++
	}
}
