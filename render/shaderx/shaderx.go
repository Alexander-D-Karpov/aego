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

// The profile is deliberately conservative. The reference compiler validates
// every generated stage and variant after these portable-surface diagnostics.
func unsupportedToken(s string) bool {
	switch s {
	case "texture2D", "texture1D", "texture1DProj", "texture2DRect", "textureQueryLevels", "textureQueryLod",
		"gl_FragColor", "gl_FragData", "gl_ClipDistance", "gl_CullDistance", "noperspective",
		"double", "dvec2", "dvec3", "dvec4", "dmat2", "dmat3", "dmat4",
		"sampler1D", "sampler1DShadow", "sampler1DArray", "sampler2DRect", "sampler2DRectShadow",
		"samplerBuffer", "isamplerBuffer", "usamplerBuffer", "sampler2DMS", "sampler2DMSArray", "samplerCubeArray",
		"image1D", "image2D", "image3D", "iimage2D", "uimage2D", "atomic_uint",
		"buffer", "shared", "packed", "coherent", "volatile", "restrict", "readonly", "writeonly", "subroutine",
		"imageLoad", "imageStore", "atomicCounter", "barrier", "memoryBarrier", "EmitVertex", "EndPrimitive":
		return true
	}
	return false
}

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
	for _, t := range tokens(strings.Join(body, "\n")) {
		if unsupportedToken(t.text) {
			return "", errs.New(errs.GPUShaderCompile, t.text+" is outside GLSL ES 3.00", log.F("shader", name), log.F("line", first+t.line+1))
		}
	}
	return Header + strings.TrimLeft(strings.Join(body, "\n"), "\n"), nil
}
