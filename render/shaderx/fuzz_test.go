package shaderx

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"aego/render/shader"
)

func FuzzPortableShaderParser(f *testing.F) {
	for _, seed := range []struct{ source, include string }{
		{head + frag, ""},
		{head + "/* uniform mat4 Fake; void vertex(){} */\n#pragma material\nuniform /* hint */ float\nValue; // @instance @default(0.5) @doc(\"a (b) \\\"c\\\"\")\n#pragma end\n" + frag, ""},
		{head + "#pragma material\n#if FLAG\nuniform float X;\n#endif\n#pragma end\n" + frag, ""},
		{head + "#pragma material\nuniform int Index; // @instance @default(2147483647)\nuniform bool Enabled; // @instance @default(true)\n#pragma end\n" + frag, ""},
		{head + "#pragma material\nuniform float X;\nuniform float X;\n#pragma end\n" + frag, ""},
		{head + "#include \"a.glsl\"\n" + frag, "#include \"b.glsl\"\n"},
		{head + "#include \"a.glsl\"\n" + frag, "#include \"a.glsl\"\n"},
		{head + "#include \"missing.glsl\"\n" + frag, ""},
		{head + "#pragma variant A B C D E F\n" + frag, ""},
	} {
		f.Add([]byte(seed.source), []byte(seed.include))
	}
	f.Fuzz(func(t *testing.T, source, include []byte) {
		// Keep random-work cost independent of the fuzz engine's corpus size.
		// Separate unit cases exercise the larger production resource limits.
		if len(source) > 60<<10 || len(include) > 60<<10 {
			t.Skip()
		}
		resolve := func(name string) (string, []byte, error) {
			switch name {
			case "a.glsl":
				return "include/a.glsl", include, nil
			case "b.glsl":
				return "include/b.glsl", []byte("float includeValue(){return 1.;}\n"), nil
			default:
				return "", nil, fmt.Errorf("missing include %s", name)
			}
		}
		u, ds := Parse(source, "fuzz.aeshader", resolve)
		if len(ds) > 64 {
			t.Fatal("diagnostics exceeded their bound")
		}
		if len(ds) != 0 {
			return
		}
		if u == nil || len(u.Params) > shader.MaxParams || len(u.Files) > shader.MaxIncludeFiles || len(u.Combos) == 0 || len(u.Combos) > shader.MaxVariants || u.Lanes > shader.MaxLanes {
			t.Fatal("accepted descriptor exceeded its declared resource budget")
		}
		for _, es := range []bool{false, true} {
			for _, fragment := range []bool{false, true} {
				source, locations := u.Source(fragment, es, u.Combos[0])
				again, repeated := u.Source(fragment, es, u.Combos[0])
				if source != again || !slices.Equal(locations, repeated) {
					t.Fatal("generation is nondeterministic")
				}
				if len(source) > shader.MaxExpandedBytes+(128<<10) || len(locations) != strings.Count(source, "\n") {
					t.Fatal("generated source or source map violated its bounds")
				}
			}
		}
	})
}
