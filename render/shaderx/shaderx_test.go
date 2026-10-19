package shaderx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const shaders = "../../engine/res/shaders"

func sources(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, pat := range []string{"*.vert", "*.frag"} {
		m, err := filepath.Glob(filepath.Join(shaders, pat))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, m...)
	}
	if len(out) == 0 {
		t.Fatal("no shaders found")
	}
	return out
}

func TestCommittedESShadersAreCurrent(t *testing.T) {
	for _, f := range sources(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		want, err := Translate(string(src), filepath.Base(f))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(shaders, ESDir, filepath.Base(f)))
		if err != nil {
			t.Fatalf("%s: run make shaders: %v", filepath.Base(f), err)
		}
		if string(got) != want {
			t.Errorf("%s is out of date, run make shaders", filepath.Base(f))
		}
	}
}

func TestESShadersValidate(t *testing.T) {
	bin, err := exec.LookPath("glslangValidator")
	if err != nil {
		t.Skip("glslangValidator is not installed")
	}
	for _, f := range sources(t) {
		es := filepath.Join(shaders, ESDir, filepath.Base(f))
		if out, err := exec.Command(bin, es).CombinedOutput(); err != nil {
			t.Errorf("%s: %s", filepath.Base(f), strings.TrimSpace(string(out)))
		}
	}
}

func TestRejectsLegacyAndForeignHeaders(t *testing.T) {
	if _, err := Translate("#version 120\nvoid main(){}", "x"); err == nil {
		t.Fatal("accepted a foreign version")
	}
	if _, err := Translate(SourceVersion+"\nvoid main(){ gl_FragColor = vec4(1.0); }", "x"); err == nil {
		t.Fatal("accepted gl_FragColor")
	}
}
