//go:build aego_golden

package golden

import (
	"image"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"aego/app/docstool"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/context"
	edocs "aego/editor/docs"
	"aego/engine"
	"aego/gpu"
	"aego/platform/input"
	"aego/render"
	"aego/render/icons"
	rendermodule "aego/render/module"
	"aego/render/text"
	"aego/render/view"
	"aego/ui/core"
	"aego/ui/draw"
	"aego/ui/theme"
)

const (
	editorW = 800
	editorH = 520
)

type editorEnv struct {
	e      *engine.Engine
	r      *render.Renderer
	ui     *core.Context
	target gpu.TextureHandle
}

func newEditorEnv(t *testing.T) *editorEnv {
	t.Helper()
	runtime.LockOSThread()
	mod := rendermodule.New(render.Options{MaxSprites: 4096})
	e, err := engine.NewBuilder(engine.Config{
		Title:    "editor-golden",
		Width:    editorW,
		Height:   editorH,
		Hidden:   true,
		ResRoot:  "../../demos/res",
		UserRoot: t.TempDir(),
		AppID:    "aego-golden-editor",
	}).WithLogger(log.Nop()).Use(mod).Build()
	if err != nil {
		t.Skipf("no GL context available: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	if name := e.Caps().Renderer; !softwareRenderer(name) && os.Getenv("AEGO_GOLDEN_ALLOW_HARDWARE") == "" {
		t.Fatalf("editor goldens require a software rasterizer, got %q", name)
	}
	r := mod.Renderer()
	ui := core.New(r.UIFont())
	ui.Theme = theme.Editor()
	set, err := text.LoadSet(e.VFS(), r.Textures(), "engine://fonts", "ui", []int{12, 14, 16, 20}, []int{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	ui.SetFonts(set)
	if mono, err := text.LoadSet(e.VFS(), r.Textures(), "engine://fonts", "ui-mono", []int{12, 14, 16, 20}, []int{1, 2}); err == nil {
		ui.SetMonoFonts(mono)
	}
	if icons, err := icons.Load(e.VFS(), r.Textures(), "engine://icons"); err == nil {
		ui.Icons = icons
	}
	target, err := e.GPU().CreateTexture(gpu.TextureDesc{Width: editorW, Height: editorH, Format: gpu.TexSRGBA8, Label: "editor.golden"})
	if err != nil {
		t.Fatal(err)
	}
	return &editorEnv{e: e, r: r, ui: ui, target: target}
}

func (env *editorEnv) draw(fn func(full mathx.Rect)) (*image.NRGBA, error) {
	ui, r, e := env.ui, env.r, env.e
	full := mathx.Rect{W: editorW, H: editorH}
	var list *draw.List
	for i := 0; i < 2; i++ {
		ui.Begin(&input.State{}, mathx.Vec2{X: editorW, Y: editorH}, 1)
		ui.Rect(full, ui.Theme.Background)
		ui.Region(full)
		fn(full)
		ui.PopCursor()
		list = ui.End()
	}
	frame := e.GPU().BeginFrame(editorW, editorH)
	if err := r.BeginFrame(frame, editorW, editorH, 1); err != nil {
		return nil, err
	}
	v := view.ScreenView(editorW, editorH, 1)
	v.Target, v.Clear, v.DoClear, v.Label = env.target, ui.Theme.Background, true, "editor.golden"
	f := r.View(v)
	white := r.White()
	if fnt := ui.Font(); fnt != nil && !fnt.White.IsZero() {
		white = fnt.White
	}
	list.Flush(f, ui.Font(), white, 0)
	f.End()
	r.EndFrame()
	e.GPU().EndFrame()
	buf := make([]byte, editorW*editorH*4)
	if err := e.GPU().ReadPixels(env.target, 0, 0, editorW, editorH, buf); err != nil {
		return nil, err
	}
	return &image.NRGBA{Pix: buf, Stride: editorW * 4, Rect: image.Rect(0, 0, editorW, editorH)}, nil
}

func checkGolden(t *testing.T, name string, img *image.NRGBA, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", name+".png")
	if *update {
		writePNG(t, path, img)
		return
	}
	compare(t, path, img, 8)
}

func TestEditorGolden(t *testing.T) {
	env := newEditorEnv(t)
	reg, err := docstool.Registry(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &context.Editor{UI: env.ui}
	for _, tc := range []struct {
		name string
		ref  string
		want string
	}{
		{"editor_docs_coroutines", "language/coroutines", "language/coroutines"},
		{"editor_docs_yield", "key:yield", "keyword/yield"},
	} {
		p := edocs.New(reg)
		p.Show(tc.ref, "")
		img, err := env.draw(func(full mathx.Rect) { p.Draw(ctx, full) })
		t.Run(tc.name, func(t *testing.T) {
			if p.Current() != tc.want {
				t.Fatalf("panel shows %q, want %q", p.Current(), tc.want)
			}
			checkGolden(t, tc.name, img, err)
		})
	}
}
