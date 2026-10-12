package main

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/projects"
	"aego/engine"
	"aego/engine/bootstrap"
)

const iconModule = `package native

import (
	"embed"
	"io/fs"

	"aego/schema"
	"aego/sdk"
)

//go:embed icons
var iconFS embed.FS

type Marker struct {
	V float32 ` + "`aego:\"v\"`" + `
}

type Module struct{}

func (Module) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "game"}
}

func (Module) Register(r *sdk.Registry) error {
	sub, err := fs.Sub(iconFS, "icons")
	if err != nil {
		return err
	}
	r.Icons(sub, "game.")
	return sdk.Component[Marker](r, schema.Name("game.Marker")).Icon("game.need").Err()
}
`

func writeIcon(t *testing.T, path string) {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIconsReportsMissingAndExtra(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the native helper")
	}
	root, err := projects.EngineRoot("")
	if err != nil {
		t.Skip(err)
	}
	p, err := projects.Create(projects.Options{Dir: filepath.Join(t.TempDir(), "icons"), Native: true, Engine: root})
	if err != nil {
		t.Fatal(err)
	}
	icons := filepath.Join(p.NativeDir(), "icons")
	os.MkdirAll(icons, 0o755)
	writeIcon(t, filepath.Join(icons, "extra@16x1.png"))
	if err := os.WriteFile(filepath.Join(p.NativeDir(), "module.go"), []byte(iconModule), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code, err := bootstrap.RunIcons(p, engine.Version, &out)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(out.String(), "game.need") || !strings.Contains(out.String(), "game.extra") {
		t.Fatalf("exit %d, output:\n%s", code, out.String())
	}
}
