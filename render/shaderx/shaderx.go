package shaderx

import (
	"strings"

	"aego/core/errs"
	"aego/core/log"
)

const (
	ESDir         = "es"
	SourceVersion = "#version 330 core"
	Header        = "#version 300 es\nprecision highp float;\nprecision highp int;\nprecision highp sampler2D;\n"
)

var unsupported = []string{"texture2D(", "gl_FragColor", "gl_FragData", "sampler1D", "noperspective", "gl_ClipDistance"}

func Translate(src, name string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	first := -1
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			first = i
			break
		}
	}
	if first < 0 {
		return "", errs.New(errs.GPUShaderCompile, "shader is empty", log.F("shader", name))
	}
	if strings.TrimSpace(lines[first]) != SourceVersion {
		return "", errs.New(errs.GPUShaderCompile, "shader must start with "+SourceVersion, log.F("shader", name))
	}
	body := lines[first+1:]
	for i, l := range body {
		for _, u := range unsupported {
			if strings.Contains(l, u) {
				return "", errs.New(errs.GPUShaderCompile, u+" has no GLSL ES 3.00 form", log.F("shader", name), log.F("line", first+i+2))
			}
		}
	}
	return Header + strings.TrimLeft(strings.Join(body, "\n"), "\n"), nil
}
