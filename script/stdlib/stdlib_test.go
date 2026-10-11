package stdlib_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/core/log"
	"aego/script/bind"
	"aego/script/diag"
	"aego/script/harness"
	"aego/script/stdlib"
	"aego/script/workspace"
)

const libTest = `package lib

import (
	"aego/log"
	"aego/math"
	"aego/rand"
	"aego/testing"
	"aego/time"
)

func TestMath() {
	testing.EqualFloat(math.Clamp(5, 0, 2), 2, 0)
	testing.EqualFloat(math.Length(Vec2{X: 3, Y: 4}), 5, 0.0001)
	testing.Equal(math.MaxInt(3, 7), 7)
	v := math.Normalize3(Vec3{Z: 2})
	testing.EqualFloat(v.Z, 1, 0.0001)
	q := math.QuatAxisAngle(Vec3{Z: 1}, math.Pi/2)
	r := math.QuatRotate(q, Vec3{X: 1})
	testing.EqualFloat(r.Y, 1, 0.0001)
}

func TestRand() {
	for i := 0; i < 200; i++ {
		n := rand.Int(3, 6)
		testing.True(n >= 3 && n < 6, "Int out of range")
		f := rand.Range(-1, 1)
		testing.True(f >= -1 && f <= 1, "Range out of range")
	}
}

func TestTimeAndLog() {
	testing.Equal(time.Frame(), 7)
	testing.EqualFloat(time.Delta(), 0.5, 0)
	log.Info("hello from " + string(time.Frame()))
}
`

func registry(t *testing.T, testingMod bool) *bind.Registry {
	t.Helper()
	r := bind.New()
	stdlib.Install(r, stdlib.Options{Seed: 1, Testing: testingMod})
	if err := r.Freeze(); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestStdlibFromScript(t *testing.T) {
	reg := registry(t, true)
	root := t.TempDir()
	p := filepath.Join(root, "scripts", "lib", "lib_test.aes")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(libTest), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.Open(workspace.Config{Root: root, Module: "game", Bindings: reg.Table()})
	if err != nil {
		t.Fatal(err)
	}
	g, ds := ws.BuildWith(workspace.BuildOptions{IncludeTests: true})
	if diag.HasErrors(ds) {
		var b bytes.Buffer
		diag.Print(&b, g.Sources, ds)
		t.Fatal(b.String())
	}
	sink := &log.MemorySink{}
	ctx := &bind.Ctx{Frame: 7, Dt: 0.5, Log: log.New(sink).With("script")}
	rep, err := harness.Run(g, reg, ctx, harness.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Failed != 0 || rep.Passed != 3 {
		var b bytes.Buffer
		rep.Print(&b, true)
		t.Fatal(b.String())
	}
	found := false
	for _, rec := range sink.Records() {
		if strings.Contains(rec.Msg, "hello from 7") {
			found = true
		}
	}
	if !found {
		t.Fatal("log.Info did not reach the logger")
	}
}

func TestStdlibHash(t *testing.T) {
	if registry(t, false).ABIHash() != registry(t, false).ABIHash() {
		t.Fatal("stdlib hash is not stable")
	}
	if registry(t, false).ABIHash() == registry(t, true).ABIHash() {
		t.Fatal("the testing module must change the hash")
	}
}
