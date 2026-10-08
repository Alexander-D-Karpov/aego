package supervise

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const nativeSrc = `package native

import (
	"aego/engine"
	aegosdk "aego/sdk"
)

type Module struct{}

func (Module) Info() engine.ModuleInfo { return engine.ModuleInfo{} }

func Modules() []aegosdk.Module { return nil }

func Extra() aegosdk.Module { return nil }

func helper() aegosdk.Module { return nil }

func WithArg(x int) aegosdk.Module { return nil }
`

func TestEntryCalls(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "module.go"), []byte(nativeSrc), 0o644)
	os.WriteFile(filepath.Join(dir, "module_test.go"), []byte("package native\n\nimport \"aego/sdk\"\n\nfunc Fake() sdk.Module { return nil }\n"), 0o644)
	got, err := entryCalls(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (entry{name: "Extra"}) || got[1] != (entry{name: "Modules", list: true}) {
		t.Fatalf("entries = %+v", got)
	}
}

func TestEntryCallsMissing(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package native\n\nfunc Nothing() {}\n"), 0o644)
	if _, err := entryCalls(dir); err == nil {
		t.Fatal("missing entry point accepted")
	}
}

func TestMainSourceParses(t *testing.T) {
	src := mainSource("example.com/game/native", []entry{{name: "Extra"}, {name: "Modules", list: true}})
	if _, err := parser.ParseFile(token.NewFileSet(), "main.go", src, 0); err != nil {
		t.Fatalf("%v\n%s", err, src)
	}
	if !strings.Contains(string(src), "native.Modules()...") || !strings.Contains(string(src), "native.Extra())") {
		t.Fatalf("source:\n%s", src)
	}
}

func TestModulePath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/game\n\ngo 1.23\n"), 0o644)
	if m, err := modulePath(dir); err != nil || m != "example.com/game" {
		t.Fatalf("module = %q err = %v", m, err)
	}
}
