package shaderx

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"aego/render/shader"
)

var update = flag.Bool("update", false, "rewrite the golden generated sources")

const testShaders = "testdata/shaders"

// testIncluder reads engine/… from the engine's include files and the rest from testdata/shaders/include.
func testIncluder(extra map[string]string) Includer {
	return func(p string) (string, []byte, error) {
		if src, ok := extra[p]; ok {
			return p, []byte(src), nil
		}
		if rest, ok := strings.CutPrefix(p, "engine/"); ok {
			b, err := os.ReadFile(filepath.Join(shaders, "include", rest))
			return "engine://shaders/include/" + rest, b, err
		}
		b, err := os.ReadFile(filepath.Join(testShaders, "include", p))
		return "shaders/include/" + p, b, err
	}
}

func parseFile(t *testing.T, name string) *Unit {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(testShaders, name))
	if err != nil {
		t.Fatal(err)
	}
	u, diags := Parse(src, name, testIncluder(nil))
	if len(diags) > 0 {
		t.Fatalf("%s: %v", name, diags)
	}
	return u
}

func testFiles(t *testing.T) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(testShaders, "*.aeshader"))
	if err != nil || len(m) != 10 {
		t.Fatalf("want 10 test shaders, have %d (%v)", len(m), err)
	}
	for i := range m {
		m[i] = filepath.Base(m[i])
	}
	return m
}

func stageName(frag, es bool) string {
	s := "gl"
	if es {
		s = "es"
	}
	if frag {
		return s + ".frag"
	}
	return s + ".vert"
}

func TestGeneratedSourcesGolden(t *testing.T) {
	for _, name := range testFiles(t) {
		u := parseFile(t, name)
		for _, es := range []bool{false, true} {
			for _, frag := range []bool{false, true} {
				src, _ := u.Source(frag, es, u.Combos[len(u.Combos)-1])
				golden := filepath.Join("testdata", "golden", strings.TrimSuffix(name, ".aeshader")+"."+stageName(frag, es))
				if *update {
					os.MkdirAll(filepath.Dir(golden), 0o755)
					if err := os.WriteFile(golden, []byte(src), 0o644); err != nil {
						t.Fatal(err)
					}
					continue
				}
				want, err := os.ReadFile(golden)
				if err != nil {
					t.Fatalf("%v: run go test ./render/shaderx -update", err)
				}
				if string(want) != src {
					t.Errorf("%s differs from the generated source; run go test ./render/shaderx -update and review the diff", golden)
				}
			}
		}
	}
}

func TestVariantsHaveTheSameLineCount(t *testing.T) {
	for _, name := range testFiles(t) {
		u := parseFile(t, name)
		for _, frag := range []bool{false, true} {
			for _, es := range []bool{false, true} {
				want := -1
				for _, mask := range u.Combos {
					src, locs := u.Source(frag, es, mask)
					n := strings.Count(src, "\n")
					if n != len(locs) {
						t.Fatalf("%s: %d lines, %d locations", name, n, len(locs))
					}
					if want >= 0 && n != want {
						t.Errorf("%s %s: variant %b has %d lines, another %d", name, stageName(frag, es), mask, n, want)
					}
					want = n
				}
			}
		}
	}
}

func TestStripKeepsLines(t *testing.T) {
	u := parseFile(t, "vertex.aeshader")
	for _, re := range []*regexp.Regexp{reFrag, reVert} {
		out := strip(u.body, re)
		if len(out) != len(u.body) {
			t.Fatalf("strip changed %d lines into %d", len(u.body), len(out))
		}
		if strings.Count(joinBody(out), "\n") != strings.Count(joinBody(u.body), "\n") {
			t.Fatal("strip changed the line count")
		}
	}
	vert, _ := u.Source(false, false, 0)
	frag, _ := u.Source(true, false, 0)
	if strings.Contains(vert, "void fragment()") || !strings.Contains(vert, "void vertex()") {
		t.Fatal("the vertex stage keeps fragment() or loses vertex()")
	}
	if strings.Contains(frag, "void vertex()") || !strings.Contains(frag, "void fragment()") {
		t.Fatal("the fragment stage keeps vertex() or loses fragment()")
	}
}

func TestDrawSlotIsMeshBlock(t *testing.T) {
	if shader.SlotDraw != 3 || !strings.Contains(meshInputs, fmt.Sprintf("uniform ub%d {", shader.SlotDraw)) {
		t.Fatalf("SlotDraw %d does not name the mesh draw block", shader.SlotDraw)
	}
}

func diagCodes(ds []Diag) []string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Code)
	}
	return out
}

const head = "#pragma aego abi 1\n"
const frag = "\nvoid fragment() { aego_out = vec4(1.0); }\n"

func TestDiagnostics(t *testing.T) {
	cases := []struct {
		name, src, code string
	}{
		{"no abi", "void fragment() {}", SHD001},
		{"no fragment", head, SHD001},
		{"unknown blend", head + "#pragma blend multiply" + frag, SHD001},
		{"pragma in include", head + "#include \"bad.glsl\"" + frag, SHD002},
		{"raw uniform", head + "uniform float X;" + frag, SHD003},
		{"bad type", head + "#pragma material\nuniform mat4 M;\n#pragma end" + frag, SHD003},
		{"unknown annotation", head + "#pragma material\nuniform float X; // @glow\n#pragma end" + frag, SHD003},
		{"long name", head + "#pragma material\nuniform float " + strings.Repeat("N", shader.MaxNameLen+1) + ";\n#pragma end" + frag, SHD004},
		{"too many textures", head + "#pragma material\n" + strings.Repeat("", 0) + textures(shader.MaxUserTextures+1) + "#pragma end" + frag, SHD004},
		{"include cycle", head + "#include \"a.glsl\"" + frag, SHD005},
		{"bad keyword", head + "#pragma variant aego_x" + frag, SHD006},
		{"unknown listed variant", head + "#pragma variant A\n#pragma variants B" + frag, SHD006},
		{"postfx", head + "#pragma domain postfx" + frag, SHD007},
		{"instance on mesh", head + "#pragma domain mesh2d\n#pragma material\nuniform float X; // @instance\n#pragma end" + frag, SHD007},
		{"legacy", head + "void fragment() { gl_FragColor = vec4(1.0); }", SHD008},
	}
	inc := testIncluder(map[string]string{
		"bad.glsl": "#pragma blend opaque\n",
		"a.glsl":   "#include \"b.glsl\"\n",
		"b.glsl":   "#include \"a.glsl\"\n",
	})
	seen := map[string]bool{}
	for _, c := range cases {
		_, diags := Parse([]byte(c.src), "t.aeshader", inc)
		if !slices.Contains(diagCodes(diags), c.code) {
			t.Errorf("%s: want %s, got %v", c.name, c.code, diags)
		}
		seen[c.code] = true
	}
	for _, code := range []string{SHD001, SHD002, SHD003, SHD004, SHD005, SHD006, SHD007, SHD008} {
		if !seen[code] {
			t.Errorf("no case for %s", code)
		}
	}
}

func textures(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "uniform sampler2D T%d;\n", i)
	}
	return b.String()
}

func TestIncludes(t *testing.T) {
	chain := map[string]string{}
	for i := 0; i < 9; i++ {
		chain[fmt.Sprintf("d%d.glsl", i)] = fmt.Sprintf("#include \"d%d.glsl\"\n", i+1)
	}
	chain["d9.glsl"] = "float deep() { return 1.0; }\n"
	_, diags := Parse([]byte(head+"#include \"d0.glsl\""+frag), "t.aeshader", testIncluder(chain))
	if !slices.ContainsFunc(diags, func(d Diag) bool { return d.Code == SHD002 && strings.Contains(d.Msg, "deeper") }) {
		t.Fatalf("depth 9 was accepted: %v", diags)
	}
	u, diags := Parse([]byte(head+"#include \"once.glsl\"\n#include \"once.glsl\""+frag), "t.aeshader", testIncluder(map[string]string{"once.glsl": "float once() { return 1.0; }\n"}))
	if len(diags) > 0 || len(u.Files) != 2 || strings.Count(joinBody(u.body), "float once()") != 1 {
		t.Fatalf("a repeated include was not skipped: files %v, diags %v", u.Files, diags)
	}
}

// An error inside an include maps back to the include's own line.
func TestMapLogPointsIntoInclude(t *testing.T) {
	inc := testIncluder(map[string]string{"broken.glsl": "float ok() { return 1.0; }\nfloat broken() { return undefinedThing; }\n"})
	u, diags := Parse([]byte(head+"#include \"broken.glsl\""+frag), "t.aeshader", inc)
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	src, locs := u.Source(true, false, 0)
	line := 0
	for i, l := range strings.Split(src, "\n") {
		if strings.Contains(l, "undefinedThing") {
			line = i + 1
		}
	}
	log := fmt.Sprintf("ERROR: 0:%d: 'undefinedThing' : undeclared identifier\n", line)
	got := MapLog(log, locs, u.Files, SHD009)
	if len(got) != 1 || got[0].File != "broken.glsl" || got[0].Line != 2 || got[0].Code != SHD009 {
		t.Fatalf("mapped to %+v", got)
	}
}

func glslangRun(t *testing.T, bin, file, src string) {
	t.Helper()
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(bin, file).CombinedOutput(); err != nil {
		t.Errorf("%s: %s", filepath.Base(file), strings.TrimSpace(string(out)))
	}
}

func TestGlslangAcceptsEveryVariant(t *testing.T) {
	bin := glslang(t)
	dir := t.TempDir()
	for _, name := range testFiles(t) {
		u := parseFile(t, name)
		for _, mask := range u.Combos {
			for _, es := range []bool{false, true} {
				for _, fr := range []bool{false, true} {
					src, _ := u.Source(fr, es, mask)
					ext := path.Ext(stageName(fr, es))
					glslangRun(t, bin, filepath.Join(dir, fmt.Sprintf("%s_%d_%s%s", strings.TrimSuffix(name, ".aeshader"), mask, strings.TrimSuffix(stageName(fr, es), ext), ext)), src)
				}
			}
		}
	}
}

// Every engine include compiles on its own inside a wrapper shader, in both stages and both dialects.
func TestGlslangAcceptsEngineIncludes(t *testing.T) {
	bin := glslang(t)
	dir := t.TempDir()
	files, _ := filepath.Glob(filepath.Join(shaders, "include", "*.glsl"))
	if len(files) == 0 {
		t.Fatal("no engine includes")
	}
	for _, f := range files {
		name := filepath.Base(f)
		u, diags := Parse([]byte(head+"#include \"engine/"+name+"\""+frag), "wrap.aeshader", testIncluder(nil))
		if len(diags) > 0 {
			t.Fatalf("%s: %v", name, diags)
		}
		for _, es := range []bool{false, true} {
			for _, fr := range []bool{false, true} {
				src, _ := u.Source(fr, es, 0)
				ext := path.Ext(stageName(fr, es))
				glslangRun(t, bin, filepath.Join(dir, strings.TrimSuffix(name, ".glsl")+"_"+strings.TrimSuffix(stageName(fr, es), ext)+ext), src)
			}
		}
	}
}
