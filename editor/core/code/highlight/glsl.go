package highlight

var glslTypes = map[string]bool{
	"void": true, "float": true, "int": true, "uint": true, "bool": true,
	"vec2": true, "vec3": true, "vec4": true, "ivec2": true, "ivec3": true, "ivec4": true,
	"uvec2": true, "uvec3": true, "uvec4": true, "bvec2": true, "bvec3": true, "bvec4": true,
	"mat2": true, "mat3": true, "mat4": true, "sampler2D": true, "samplerCube": true,
	"isampler2D": true, "usampler2D": true,
}
var glslWords = map[string]bool{
	"layout": true, "uniform": true, "in": true, "out": true, "inout": true,
	"flat": true, "smooth": true, "centroid": true, "precision": true, "highp": true, "mediump": true, "lowp": true,
	"const": true, "struct": true, "if": true, "else": true, "for": true, "while": true, "do": true,
	"break": true, "continue": true, "return": true, "discard": true, "switch": true, "case": true, "default": true,
	"true": true, "false": true, "version": true, "include": true, "define": true, "undef": true,
	"ifdef": true, "ifndef": true, "elif": true, "endif": true, "extension": true, "line": true,
}

// GLSLLine shares comment/string/number handling with the code editor lexer;
// only identifier classification differs. It is syntax coloring, not validation.
func GLSLLine(text []byte, in State, dst []Span) ([]Span, State) {
	spans, state := Line(text, in, dst)
	for i := range spans {
		s := &spans[i]
		switch s.Kind {
		case Comment, String, Number, Bracket, Operator, Punctuation:
			continue
		}
		if glslTypes[string(text[s.Start:s.End])] {
			s.Kind = BuiltinType
		} else if glslWords[string(text[s.Start:s.End])] {
			s.Kind = Keyword
		}
	}
	return spans, state
}
