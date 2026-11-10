//go:build aego_gltest

package agent

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"aego/core/log"
	"aego/core/mathx"
	"aego/engine"
	"aego/gpu"
	"aego/platform/input"
	"aego/render"
	"aego/render/text"
	"aego/render/texture"
	"aego/render/view"
	"aego/ui/core"
	"aego/ui/draw"
	"aego/ui/richtext"
	"aego/ui/theme"
)

const cullPixelW, cullPixelH = 480, 320

type cullPixelEnv struct {
	e      *engine.Engine
	r      *render.Renderer
	target gpu.TextureHandle
	fonts  *text.FontSet
	mono   *text.FontSet
	shapes *draw.Shapes
}

func newCullPixelEnv(t *testing.T) *cullPixelEnv {
	t.Helper()
	e, err := engine.NewBuilder(engine.Config{Title: "agent-cull-pixels", Width: cullPixelW, Height: cullPixelH, Hidden: true, ResRoot: t.TempDir(), UserRoot: t.TempDir(), AppID: "agent-cull-pixels"}).WithLogger(log.Nop()).Build()
	if err != nil {
		t.Fatal(err)
	}
	if e.Caps().Renderer == "null" {
		e.Close()
		t.Fatal("real GL context required; refusing a null renderer")
	}
	r, err := render.New(e.GPU(), e.Caps(), e.VFS(), log.Nop(), render.Options{MaxSprites: 16384, MaxMeshes: 4, MaxSprites3D: 1})
	if err != nil {
		e.Close()
		t.Fatal(err)
	}
	target, err := e.GPU().CreateTexture(gpu.TextureDesc{Width: cullPixelW, Height: cullPixelH, Format: gpu.TexSRGBA8, Label: "agent.cull.pixels"})
	if err != nil {
		r.Close()
		e.Close()
		t.Fatal(err)
	}
	ready := false
	defer func() {
		if !ready {
			e.GPU().DestroyTexture(target)
			r.Close()
			e.Close()
		}
	}()
	fonts, err := text.LoadSet(e.VFS(), r.Textures(), "engine://fonts", "ui", []int{12, 14, 16, 20}, []int{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	mono, err := text.LoadSet(e.VFS(), r.Textures(), "engine://fonts", "ui-mono", []int{12, 14, 16, 20}, []int{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	shapes, err := draw.NewShapes(e.GPU(), r.Textures())
	if err != nil {
		t.Fatal(err)
	}
	ready = true
	t.Logf("actual GL renderer: %s", e.Caps().Renderer)
	return &cullPixelEnv{e: e, r: r, target: target, fonts: fonts, mono: mono, shapes: shapes}
}

func (env *cullPixelEnv) close() {
	env.e.GPU().DestroyTexture(env.target)
	env.r.Close()
	env.e.Close()
}

type cullPixelHarness struct {
	ui        *core.Context
	p         *Panel
	in        input.State
	clipboard testTranscriptClipboard
	body      mathx.Rect
	track     mathx.Rect
	scale     float32
	opened    []string
}

func (env *cullPixelEnv) harness(t *testing.T, th theme.Theme, scale float32, md string, enabled bool, font *text.Font) *cullPixelHarness {
	t.Helper()
	h := &cullPixelHarness{ui: core.New(env.r.UIFont()), scale: scale}
	h.ui.Theme = th
	h.ui.Clipboard = &h.clipboard
	h.ui.SetShapes(env.shapes)
	if font == nil {
		h.ui.SetFonts(env.fonts)
		h.ui.SetMonoFonts(env.mono)
	} else {
		h.ui.SetFont(font)
	}
	w, height := float32(cullPixelW)/scale, float32(cullPixelH)/scale
	h.body = mathx.Rect{X: 12, Y: 12, W: w - 38, H: height - 24}
	h.track = mathx.Rect{X: h.body.X + h.body.W + 2, Y: h.body.Y, W: th.ScrollbarW, H: h.body.H}
	h.p = New(Deps{Host: newProviderTestHost(), Open: func(s string) { h.opened = append(h.opened, s) }})
	h.p.stick = false
	h.ui.Begin(&h.in, mathx.Vec2{X: w, Y: height}, scale)
	b := h.p.push(block{kind: blockText, text: md})
	width := max(float32(40), h.body.W-2*th.Padding)
	b.h = h.p.measure(h.ui, b, width)
	b.hw = width
	b.md.CullClipped = enabled
	h.ui.End()
	return h
}

func (h *cullPixelHarness) maxScroll() float32 {
	th := &h.ui.Theme
	return max(0, th.Padding+h.p.blocks[0].h+2*th.Spacing+th.Padding-h.body.H)
}

func (h *cullPixelHarness) scrollFor(line *richtext.Line, relativeY float32) float32 {
	return mathx.Clamp(3*h.ui.Theme.Padding+line.Y-relativeY, 0, h.maxScroll())
}

func (h *cullPixelHarness) lineY(line *richtext.Line) float32 {
	return h.body.Y + 3*h.ui.Theme.Padding - h.p.scroll + line.Y
}

func (h *cullPixelHarness) findLine(t *testing.T, kind richtext.LineKind, contains, target string) (*richtext.Line, *richtext.Run) {
	t.Helper()
	for i := range h.p.blocks[0].md.Doc().Lines {
		line := &h.p.blocks[0].md.Doc().Lines[i]
		if line.Kind != kind {
			continue
		}
		for j := range line.Runs {
			run := &line.Runs[j]
			if strings.Contains(run.Text, contains) && run.Target == target {
				return line, run
			}
		}
	}
	t.Fatalf("missing line kind=%v text=%q target=%q", kind, contains, target)
	return nil, nil
}

type cullPixelFrame struct {
	pixels []byte
	lines  int
}

func (env *cullPixelEnv) frame(t *testing.T, h *cullPixelHarness, clip mathx.Rect) cullPixelFrame {
	t.Helper()
	ui := h.ui
	size := mathx.Vec2{X: float32(cullPixelW) / h.scale, Y: float32(cullPixelH) / h.scale}
	ui.Begin(&h.in, size, h.scale)
	ui.Rect(mathx.Rect{W: size.X, H: size.Y}, ui.Theme.Background)
	ui.Rect(h.body, ui.Theme.Panel)
	ui.PushClip(clip)
	h.p.transcript(ui, h.body, h.track)
	ui.PopClip()
	list := ui.End()
	// The input intentionally contains many offscreen LineNN paragraphs; this
	// confirms the paired pixels exercise the optimized path, not merely a toggle.
	lines := len(list.TestDrawnItems(mathx.Rect{W: size.X, H: size.Y}, ui.Font(), "Line", 0, 0))
	if err := env.r.BeginFrame(env.e.GPU().BeginFrame(cullPixelW, cullPixelH), cullPixelW, cullPixelH, h.scale); err != nil {
		t.Fatal(err)
	}
	v := view.ScreenView(cullPixelW, cullPixelH, h.scale)
	v.Target, v.Clear, v.DoClear = env.target, ui.Theme.Background, true
	f := env.r.View(v)
	list.Flush(f, ui.Font(), env.r.White(), 0)
	f.End()
	env.r.EndFrame()
	env.e.GPU().EndFrame()
	if dropped := env.r.Stats().Dropped; dropped != 0 {
		t.Fatalf("renderer dropped %d sprites", dropped)
	}
	pixels := make([]byte, cullPixelW*cullPixelH*4)
	if err := env.e.GPU().ReadPixels(env.target, 0, 0, cullPixelW, cullPixelH, pixels); err != nil {
		t.Fatal(err)
	}
	return cullPixelFrame{pixels: pixels, lines: lines}
}

func compareCullPixels(t *testing.T, name string, before, after cullPixelFrame) {
	t.Helper()
	if !bytes.Equal(before.pixels, after.pixels) {
		different, first := 0, -1
		for i := 0; i < len(before.pixels); i += 4 {
			if !bytes.Equal(before.pixels[i:i+4], after.pixels[i:i+4]) {
				different++
				if first < 0 {
					first = i
				}
			}
		}
		t.Fatalf("%s: %d differing pixels, first=(%d,%d) before=%v after=%v", name, different, (first/4)%cullPixelW, (first/4)/cullPixelW, before.pixels[first:first+4], after.pixels[first:first+4])
	}
	t.Logf("%s: exact RGBA sha256=%x Line commands=%d/%d", name, sha256.Sum256(before.pixels), before.lines, after.lines)
}

func cullPixelMarkdown() string {
	var b strings.Builder
	for i := range 12 {
		fmt.Fprintf(&b, "Line%02d Alpha beta **bold** and `inline` words.\n\n", i)
	}
	b.WriteString("[earlier](ref:earlier) and [target link](ref:target)\n\n")
	b.WriteString("SELECT alpha beta gamma delta epsilon zeta\n\n")
	b.WriteString("```aes run\ncode_first()\ncode_second()\ncode_last()\n```\n\n")
	for i := 12; i < 26; i++ {
		fmt.Fprintf(&b, "Line%02d trailing text remains scrollable.\n\n", i)
	}
	return b.String()
}

func newCullPair(t *testing.T, env *cullPixelEnv, th theme.Theme, scale float32, md string) (*cullPixelHarness, *cullPixelHarness) {
	t.Helper()
	return env.harness(t, th, scale, md, false, nil), env.harness(t, th, scale, md, true, nil)
}

func TestAgentCullClippedRenderedPixels(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	env := newCullPixelEnv(t)
	defer env.close()
	for _, tc := range []struct {
		name string
		th   theme.Theme
	}{{"dark", theme.Editor()}, {"light", theme.Light()}} {
		for _, scale := range []float32{1, 2} {
			before, after := newCullPair(t, env, tc.th, scale, cullPixelMarkdown())
			code, _ := before.findLine(t, richtext.CodeLine, "code_first", "")
			body := before.body
			inner := mathx.Rect{X: body.X + 2, Y: body.Y + 20, W: body.W - 4, H: body.H - 40}
			cases := []struct {
				name   string
				scroll float32
				clip   mathx.Rect
			}{
				{"top", 0, body},
				{"code-crosses-top", before.scrollFor(code, inner.Y-body.Y-3), inner},
				{"code-crosses-bottom", before.scrollFor(code, inner.Y-body.Y+inner.H-5), inner},
				{"horizontal-clip", before.scrollFor(code, body.H/2), mathx.Rect{X: body.X + body.W*0.3, Y: body.Y + 15, W: body.W * 0.4, H: body.H - 30}},
				{"bottom", before.maxScroll(), mathx.Rect{X: body.X, Y: body.Y + body.H/2, W: body.W, H: body.H / 2}},
			}
			culled := false
			for _, c := range cases {
				before.p.scroll, after.p.scroll = c.scroll, c.scroll
				before.in.BeginFrame()
				after.in.BeginFrame()
				old, now := env.frame(t, before, c.clip), env.frame(t, after, c.clip)
				compareCullPixels(t, fmt.Sprintf("%s/scale%.0f/%s", tc.name, scale, c.name), old, now)
				culled = culled || old.lines > now.lines
			}
			if !culled {
				t.Fatalf("%s scale %.0f: no offscreen transcript commands were culled", tc.name, scale)
			}
			checkCullCodeBackground(t, env, before, after, tc.name, scale)
			checkCullSelection(t, env, tc.th, scale, tc.name)
			checkCullLinks(t, env, tc.th, scale, tc.name)
		}
		checkCullOverhang(t, env, tc.th, tc.name)
	}
}

func checkCullCodeBackground(t *testing.T, env *cullPixelEnv, before, after *cullPixelHarness, name string, scale float32) {
	t.Helper()
	code, _ := before.findLine(t, richtext.CodeLine, "code_first", "")
	before.p.scroll = before.scrollFor(code, before.body.H/2)
	after.p.scroll = before.p.scroll
	before.in.BeginFrame()
	after.in.BeginFrame()
	old, now := env.frame(t, before, before.body), env.frame(t, after, after.body)
	compareCullPixels(t, fmt.Sprintf("%s/scale%.0f/code-background", name, scale), old, now)
	x := int((before.body.X + before.body.W - 30) * scale)
	y := int((before.lineY(code) + code.H/2) * scale)
	outY := int((before.lineY(code) - richtext.CodePad - 2) * scale)
	pixel := func(y int) []byte { i := (y*cullPixelW + x) * 4; return old.pixels[i : i+4] }
	if x < 0 || x >= cullPixelW || outY < 0 || y >= cullPixelH || bytes.Equal(pixel(y), pixel(outY)) {
		t.Fatalf("%s/scale%.0f code background not visible: x=%d y=%d outside=%d", name, scale, x, y, outY)
	}
}

func checkCullSelection(t *testing.T, env *cullPixelEnv, th theme.Theme, scale float32, name string) {
	t.Helper()
	before, after := newCullPair(t, env, th, scale, cullPixelMarkdown())
	line, run := before.findLine(t, richtext.TextLine, "SELECT", "")
	before.p.scroll = before.scrollFor(line, before.body.H/2)
	after.p.scroll = before.p.scroll
	clip := before.body
	start := mathx.Vec2{X: before.body.X + 3*th.Padding + run.X + 2, Y: before.lineY(line) + line.H/2}
	end := mathx.Vec2{X: start.X + min(run.W-4, float32(75)), Y: start.Y}
	before.in.BeginFrame()
	after.in.BeginFrame()
	plain := env.frame(t, before, clip)
	compareCullPixels(t, fmt.Sprintf("%s/scale%.0f/selection-empty", name, scale), plain, env.frame(t, after, clip))
	for stage := range 3 {
		for _, h := range []*cullPixelHarness{before, after} {
			h.in.BeginFrame()
			switch stage {
			case 0:
				h.in.ApplyMouseMove(start, mathx.Vec2{})
				h.in.ApplyMouseDown(start, input.MouseLeft)
			case 1:
				h.in.ApplyMouseMove(end, mathx.Vec2{X: end.X - start.X})
			case 2:
				h.in.ApplyMouseUp(end, input.MouseLeft)
			}
		}
		old, now := env.frame(t, before, clip), env.frame(t, after, clip)
		compareCullPixels(t, fmt.Sprintf("%s/scale%.0f/selection-stage%d", name, scale, stage), old, now)
		if stage == 2 && bytes.Equal(plain.pixels, old.pixels) {
			t.Fatalf("%s/scale%.0f selection produced no visible pixels", name, scale)
		}
	}
	if !before.p.textPick.set || before.p.textPick.a == before.p.textPick.b || before.p.textPick.a != after.p.textPick.a || before.p.textPick.b != after.p.textPick.b {
		t.Fatalf("%s/scale%.0f selection differs: before=%+v after=%+v", name, scale, before.p.textPick, after.p.textPick)
	}
	for _, h := range []*cullPixelHarness{before, after} {
		h.in.BeginFrame()
		h.in.ApplyKeyDown(input.KeyC, 0, input.PrimaryMod(), false)
	}
	compareCullPixels(t, fmt.Sprintf("%s/scale%.0f/selection-copy", name, scale), env.frame(t, before, clip), env.frame(t, after, clip))
	if before.clipboard.value == "" || before.clipboard.value != after.clipboard.value {
		t.Fatalf("%s/scale%.0f copied selections differ: %q vs %q", name, scale, before.clipboard.value, after.clipboard.value)
	}
}

func checkCullLinks(t *testing.T, env *cullPixelEnv, th theme.Theme, scale float32, name string) {
	t.Helper()
	for _, cancel := range []bool{false, true} {
		before, after := newCullPair(t, env, th, scale, cullPixelMarkdown())
		line, run := before.findLine(t, richtext.TextLine, "target", "ref:target")
		before.p.scroll = before.scrollFor(line, before.body.H/2)
		after.p.scroll = before.p.scroll
		at := mathx.Vec2{X: before.body.X + 3*th.Padding + run.X + 2, Y: before.lineY(line) + line.H/2}
		narrow := mathx.Rect{X: before.body.X, Y: before.lineY(line) - 2, W: before.body.W, H: line.H + 4}
		pressClip, releaseClip := narrow, before.body
		if cancel {
			pressClip = before.body
			releaseClip = mathx.Rect{X: before.body.X, Y: before.body.Y, W: before.body.W, H: 10}
		}
		for _, h := range []*cullPixelHarness{before, after} {
			h.in.BeginFrame()
			h.in.ApplyMouseMove(at, mathx.Vec2{})
			h.in.ApplyMouseDown(at, input.MouseLeft)
		}
		compareCullPixels(t, fmt.Sprintf("%s/scale%.0f/link-press-cancel%t", name, scale, cancel), env.frame(t, before, pressClip), env.frame(t, after, pressClip))
		if before.ui.Active() == 0 || before.ui.Active() != after.ui.Active() {
			t.Fatalf("%s/scale%.0f link press did not acquire matching active IDs", name, scale)
		}
		for _, h := range []*cullPixelHarness{before, after} {
			h.in.BeginFrame()
			h.in.ApplyMouseUp(at, input.MouseLeft)
		}
		compareCullPixels(t, fmt.Sprintf("%s/scale%.0f/link-release-cancel%t", name, scale, cancel), env.frame(t, before, releaseClip), env.frame(t, after, releaseClip))
		if before.ui.Active() != 0 || after.ui.Active() != 0 || strings.Join(before.opened, ",") != strings.Join(after.opened, ",") {
			t.Fatalf("%s/scale%.0f link release differs: active=%v/%v opened=%v/%v", name, scale, before.ui.Active(), after.ui.Active(), before.opened, after.opened)
		}
		if !cancel && strings.Join(before.opened, ",") != "ref:target" || cancel && len(before.opened) != 0 {
			t.Fatalf("%s/scale%.0f link result cancel=%t: %v", name, scale, cancel, before.opened)
		}
	}
}

func cullOverhangFont(source *text.Font, omit rune) *text.Font {
	f := text.NewFont(source.Size, source.Scale, source.LineHeight, source.Ascent)
	f.Name, f.Texture, f.White = source.Name, source.Texture, source.White
	for r := rune(32); r < 127; r++ {
		g, ok := source.Glyph(r)
		if !ok {
			continue
		}
		if r == 'A' {
			g.Offset.Y += 90
		}
		if r == 'Z' {
			g.Offset.Y -= 90
		}
		if r == omit {
			g.Region = texture.Region{}
		}
		f.SetGlyph(r, g)
	}
	return f
}

func checkCullOverhang(t *testing.T, env *cullPixelEnv, th theme.Theme, name string) {
	t.Helper()
	md := "A\n\n" + strings.Repeat("Line filler\n\n", 20) + "Z"
	for _, glyph := range []rune{'A', 'Z'} {
		font := cullOverhangFont(env.r.UIFont(), 0)
		controlFont := cullOverhangFont(env.r.UIFont(), glyph)
		before := env.harness(t, th, 1, md, false, font)
		after := env.harness(t, th, 1, md, true, font)
		control := env.harness(t, th, 1, md, false, controlFont)
		line, run := before.findLine(t, richtext.TextLine, string(glyph), "")
		if glyph == 'Z' {
			before.p.scroll = before.maxScroll()
		}
		after.p.scroll, control.p.scroll = before.p.scroll, before.p.scroll
		g, ok := font.Glyph(glyph)
		if !ok || g.Region.IsZero() {
			t.Fatalf("%s overhang glyph %q is unavailable", name, glyph)
		}
		glyphY := before.lineY(line) + (line.H-font.LineHeight)/2 + g.Offset.Y
		clip := mathx.Rect{X: before.body.X + 3*th.Padding + run.X - 1, Y: glyphY + 1, W: g.Size.X + 2, H: max(float32(1), g.Size.Y-2)}
		if clip.Y < before.body.Y || clip.Y+clip.H > before.body.Y+before.body.H || (glyph == 'A' && before.lineY(line)+line.H+richtext.CodePad >= clip.Y) || (glyph == 'Z' && before.lineY(line)-richtext.CodePad <= clip.Y+clip.H) {
			t.Fatalf("%s glyph %q did not cross from outside parent clip: line=%v clip=%+v body=%+v", name, glyph, before.lineY(line), clip, before.body)
		}
		before.in.BeginFrame()
		after.in.BeginFrame()
		control.in.BeginFrame()
		old, now, absent := env.frame(t, before, clip), env.frame(t, after, clip), env.frame(t, control, clip)
		compareCullPixels(t, fmt.Sprintf("%s/overhang-%c", name, glyph), old, now)
		if bytes.Equal(old.pixels, absent.pixels) {
			t.Fatalf("%s glyph %q is not actually visible inside the parent clip", name, glyph)
		}
	}
}
