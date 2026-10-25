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
	"aego/editor/core/context"
	"aego/editor/core/services"
	edocs "aego/editor/panels/docs"
	"aego/engine"
	"aego/engine/docs"
	"aego/gpu"
	"aego/platform/input"
	"aego/render"
	renderhost "aego/render/host"
	"aego/render/icons"
	"aego/render/text"
	"aego/render/view"
	"aego/ui/core"
	"aego/ui/draw"
	"aego/ui/theme"
	"aego/ui/widgets"
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
	w, h   int
}

func newEditorEnv(t *testing.T) *editorEnv {
	return newEditorEnvSize(t, editorW, editorH)
}

func newEditorEnvSize(t *testing.T, w, h int) *editorEnv {
	t.Helper()
	return newEditorEnvTheme(t, w, h, theme.Editor())
}

func newEditorEnvTheme(t *testing.T, w, h int, th theme.Theme) *editorEnv {
	t.Helper()
	runtime.LockOSThread()
	mod := renderhost.New(render.Options{MaxSprites: 65536})
	e, err := engine.NewBuilder(engine.Config{
		Title:    "editor-golden",
		Width:    w,
		Height:   h,
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
	ui.Theme = th
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
	if sh, err := draw.NewShapes(e.GPU(), r.Textures()); err == nil {
		ui.SetShapes(sh)
	}
	target, err := e.GPU().CreateTexture(gpu.TextureDesc{Width: w, Height: h, Format: gpu.TexSRGBA8, Label: "editor.golden"})
	if err != nil {
		t.Fatal(err)
	}
	return &editorEnv{e: e, r: r, ui: ui, target: target, w: w, h: h}
}

func (env *editorEnv) draw(fn func(full mathx.Rect)) (*image.NRGBA, error) {
	ui, r, e := env.ui, env.r, env.e
	w, h := env.w, env.h
	full := mathx.Rect{W: float32(w), H: float32(h)}
	var list *draw.List
	for i := 0; i < 2; i++ {
		ui.Begin(&input.State{}, mathx.Vec2{X: float32(w), Y: float32(h)}, 1)
		ui.Rect(full, ui.Theme.Background)
		ui.Region(full)
		fn(full)
		ui.PopCursor()
		list = ui.End()
	}
	frame := e.GPU().BeginFrame(w, h)
	if err := r.BeginFrame(frame, w, h, 1); err != nil {
		return nil, err
	}
	v := view.ScreenView(w, h, 1)
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
	buf := make([]byte, w*h*4)
	if err := e.GPU().ReadPixels(env.target, 0, 0, w, h, buf); err != nil {
		return nil, err
	}
	return &image.NRGBA{Pix: buf, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}, nil
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
		{"editor_docs_statements", "language/declarations-and-statements", "language/declarations-and-statements"},
		{"editor_docs_types", "language/types", "language/types"},
	} {
		p, img, err := shotDocs(env, reg, ctx, tc.ref)
		t.Run(tc.name, func(t *testing.T) {
			if p.Current() != tc.want {
				t.Fatalf("panel shows %q, want %q", p.Current(), tc.want)
			}
			checkGolden(t, tc.name, img, err)
		})
	}
}

func shotDocs(env *editorEnv, reg *docs.Registry, ctx *context.Editor, ref string) (*edocs.Panel, *image.NRGBA, error) {
	p := edocs.New(reg, ctx, ctx)
	p.Show(ref, "")
	img, err := env.draw(func(full mathx.Rect) { p.Draw(&services.Frame{UI: env.ui}, full) })
	return p, img, err
}

func TestEditorMenuGolden(t *testing.T) {
	img, err := shotMenu(t, newEditorEnv(t))
	checkGolden(t, "editor_menu_popup", img, err)
}

func shotMenu(t *testing.T, env *editorEnv) (*image.NRGBA, error) {
	t.Helper()
	ui := env.ui
	id := core.IDOf("golden.menu")
	img, err := env.draw(func(full mathx.Rect) {
		if !ui.PopupOpen(id) {
			ui.OpenPopup(id, mathx.Vec2{X: 40, Y: 40})
		}
		if ui.BeginPopup(id, 240) {
			widgets.Entry(ui, widgets.Item{Label: "Frame Selection", Shortcut: "F", Icon: "view.zoom", Enabled: true})
			widgets.Entry(ui, widgets.Item{Label: "Snap", Icon: "view.snap", Enabled: true, Checked: true})
			widgets.Separator(ui)
			widgets.Entry(ui, widgets.Item{Label: "Delete", Shortcut: "Delete"})
			ui.EndPopup()
		}
		widgets.ButtonRect(ui, ui.ID("btn"), mathx.Rect{X: 320, Y: 40, W: 120, H: 26}, "Button")
		v := "field"
		widgets.TextFieldRect(ui, ui.ID("fld"), mathx.Rect{X: 320, Y: 80, W: 160, H: 24}, &v, false)
	})
	return img, err
}
