//go:build aego_golden

package golden

import (
	"image"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/app/edit"
	"aego/contracts/debugdraw"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/play"
	"aego/editor/core/services"
	"aego/editor/panels/events"
	"aego/editor/panels/watch"
	"aego/engine"
	"aego/engine/recorder"
	"aego/gpu"
	"aego/modules/debug"
	"aego/modules/render2d"
	"aego/project"
	"aego/render/rt"
	"aego/render/view"
	"aego/scene"
	"aego/scene/sceneio"
)

func goldenTypes(t *testing.T) *scene.Types {
	t.Helper()
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Default().Load(types, nil, assembly.Options{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return types
}

func cameraScene(s *scene.Scene) {
	cam := s.Create(s.Root(), "Camera")
	c := render2d.DefaultCamera2D()
	c.Zoom = 24
	*scene.Add[render2d.Camera2D](s, cam) = c
	s.SetPosition(cam, mathx.Vec2{Y: 1})
	n := s.Create(s.Root(), "Player")
	sr := scene.Add[render2d.SpriteRenderer](s, n)
	*sr = render2d.DefaultSpriteRenderer()
	sr.Size = mathx.Vec2{X: 1, Y: 2}
}

func playSession(t *testing.T, w, h int, build func(*scene.Scene)) (*edit.Session, *play.Controller) {
	t.Helper()
	runtime.LockOSThread()
	p, err := project.Create(t.TempDir(), "DebugGolden", false)
	if err != nil {
		t.Fatal(err)
	}
	src := scene.New(goldenTypes(t), nil, scene.Options{})
	build(src)
	src.Flush()
	rel := "scenes/golden.aescene"
	if err := os.WriteFile(filepath.Join(p.Root, filepath.FromSlash(rel)), sceneio.Save(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Hidden: true, NoWatch: true, Width: w, Height: h, Logger: log.Nop()})
	if err != nil {
		t.Skipf("no GL context available: %v", err)
	}
	t.Cleanup(s.Close)
	if name := s.Engine.Caps().Renderer; !softwareRenderer(name) && os.Getenv("AEGO_GOLDEN_ALLOW_HARDWARE") == "" {
		t.Fatalf("goldens require a software rasterizer, got %q", name)
	}
	s.Step()
	d, err := s.App.Context().Docs.Open(rel)
	if err != nil {
		t.Fatal(err)
	}
	pc := s.App.Play()
	if err := pc.Start(d); err != nil {
		t.Fatal(err)
	}
	return s, pc
}

func TestEditorPlayToolbar(t *testing.T) {
	s, pc := playSession(t, editorW, editorH, cameraScene)
	pc.Pause()
	pc.SetTimeScale(0.5)
	for range 2 {
		pc.StepFixed()
		pc.Tick(1.0 / 60)
	}
	if pc.State() != play.Paused || pc.Ticks() != 2 {
		t.Fatalf("state %v ticks %d, want paused on tick 2", pc.State(), pc.Ticks())
	}
	s.Step()
	s.Step()
	e := s.Engine
	m := e.Metrics()
	w, h := m.PixelW, m.PixelH
	ui := s.App.Context().UI
	tgt, err := e.GPU().CreateTexture(gpu.TextureDesc{Width: w, Height: h, Format: gpu.TexSRGBA8, Label: "golden.toolbar"})
	if err != nil {
		t.Fatal(err)
	}
	frame := e.GPU().BeginFrame(w, h)
	if err := s.Renderer.BeginFrame(frame, w, h, 1); err != nil {
		t.Fatal(err)
	}
	v := view.ScreenView(w, h, m.Density())
	v.Target, v.Clear, v.DoClear, v.Label = tgt, ui.Theme.Background, true, "golden.toolbar"
	f := s.Renderer.View(v)
	white := s.Renderer.White()
	if fnt := ui.Font(); fnt != nil && !fnt.White.IsZero() {
		white = fnt.White
	}
	s.App.UIList().Flush(f, ui.Font(), white, 0)
	f.End()
	s.Renderer.EndFrame()
	e.GPU().EndFrame()
	buf := make([]byte, w*h*4)
	err = e.GPU().ReadPixels(tgt, 0, 0, w, h, buf)
	full := &image.NRGBA{Pix: buf, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
	dens := float32(m.Density())
	top := int(2 * ui.Theme.BarHeight * dens)
	bottom := int(ui.Theme.MenuHeight * dens)
	img := image.NewNRGBA(image.Rect(0, 0, w, top+bottom))
	draw.Draw(img, image.Rect(0, 0, w, top), full, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(0, top, w, top+bottom), full, image.Point{Y: h - bottom}, draw.Src)
	checkGolden(t, "editor_play_toolbar", img, err)
}

func eventWorld(t *testing.T) *engine.World {
	t.Helper()
	s := scene.New(goldenTypes(t), nil, scene.Options{})
	player := ids.NodeID{Hi: 0x1001, Lo: 1}
	coin := ids.NodeID{Hi: 0x1002, Lo: 2}
	enemy := ids.NodeID{Hi: 0x1003, Lo: 3}
	spark := ids.NodeID{Hi: 0x5a7c3e91d2b4f608, Lo: 4}
	s.CreateWithID(s.Root(), "Player", player)
	s.CreateWithID(s.Root(), "Coin", coin)
	s.CreateWithID(s.Root(), "Enemy", enemy)
	s.Flush()
	w := engine.NewWorld("golden")
	w.SetService(scene.ServiceID, s)
	rec := recorder.New(64)
	w.Recorder = rec
	rec.Stamp(1, 10, 20)
	rec.Add(recorder.Signal, player, ids.NodeID{}, "Jumped", "PlayerController")
	rec.Add(recorder.Event, player, coin, "OnTriggerEnter", "")
	rec.Add(recorder.Fault, enemy, ids.NodeID{}, "mygame.enemy.EnemyAI", "division by zero")
	rec.Stamp(1, 11, 21)
	rec.Add(recorder.Spawn, spark, ids.NodeID{}, "Spark", "")
	rec.Add(recorder.Despawn, coin, ids.NodeID{}, "Coin", "")
	rec.Count(recorder.Event)
	return w
}

func TestEditorEventMonitor(t *testing.T) {
	env := newEditorEnvSize(t, 640, 220)
	w := eventWorld(t)
	p := events.New(events.Deps{World: func() *engine.World { return w }})
	img, err := env.draw(func(full mathx.Rect) { p.Draw(&services.Frame{UI: env.ui}, full) })
	checkGolden(t, "editor_event_monitor", img, err)
	p.Focus(10)
	img, err = env.draw(func(full mathx.Rect) { p.Draw(&services.Frame{UI: env.ui}, full) })
	checkGolden(t, "editor_event_monitor_frame", img, err)
}

func TestEditorWatch(t *testing.T) {
	env := newEditorEnvSize(t, 480, 140)
	s := scene.New(goldenTypes(t), nil, scene.Options{})
	cameraScene(s)
	s.Flush()
	w := engine.NewWorld("golden")
	w.SetService(scene.ServiceID, s)
	for range 3 {
		plan := w.Time.Begin(1.0 / 60)
		for range plan.Steps {
			w.Time.BeginTick()
		}
	}
	p := watch.New(watch.Deps{World: func() *engine.World { return w }})
	p.SetExprs([]string{"Player.SpriteRenderer.Size", "Ghost.SpriteRenderer.Size.y", "time.Tick"})
	img, err := env.draw(func(full mathx.Rect) { p.Draw(&services.Frame{UI: env.ui}, full) })
	checkGolden(t, "editor_watch", img, err)
}

func TestGameViewDebugDraw(t *testing.T) {
	s, pc := playSession(t, size, size, cameraScene)
	pc.Tick(1.0 / 60)
	debugPrims(t, pc)
	e := s.Engine
	pool := rt.New(e.GPU(), gpu.TexSRGBA8, "golden.debugdraw")
	tgt, err := pool.Resize(nil, size, size, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		pool.Release(tgt)
		pool.Close()
	}()
	frame := e.GPU().BeginFrame(size, size)
	if err := s.Renderer.BeginFrame(frame, size, size, 1); err != nil {
		t.Fatal(err)
	}
	pc.Render(tgt)
	s.Renderer.EndFrame()
	e.GPU().EndFrame()
	buf := make([]byte, size*size*4)
	err = e.GPU().ReadPixels(tgt.Texture, 0, 0, size, size, buf)
	img := &image.NRGBA{Pix: buf, Stride: size * 4, Rect: image.Rect(0, 0, size, size)}
	checkGolden(t, "gameview_debug_draw", img, err)
}

var (
	arrowFrom = mathx.Vec2{X: -3, Y: 3}
	arrowTo   = mathx.Vec2{X: 1, Y: 0}
	debugCam  = mathx.Vec2{Y: 1}
	ringAt    = mathx.Vec2{X: 3, Y: -2.5}
)

func debugPrims(t *testing.T, pc *play.Controller) *debug.Service {
	t.Helper()
	d := debug.Of(pc.World())
	if d == nil {
		t.Fatal("the play world has no debug draw service")
	}
	d.Circle(mathx.Vec2{X: -2, Y: 1}, 1.5, &debugdraw.Style{Color: mathx.Color{G: 1, B: 1, A: 1}, Life: debugdraw.ForSeconds(2), Fade: debugdraw.FadeAlpha})
	d.Advance(false, 1, 1)
	d.Line(mathx.Vec2{X: -4, Y: -1}, mathx.Vec2{X: 4, Y: -1}, &debugdraw.Style{Color: mathx.Color{R: 1, A: 1}})
	d.Arrow(arrowFrom, arrowTo, &debugdraw.Style{Color: mathx.Color{G: 1, A: 1}, Thickness: 3})
	d.Rect(mathx.Rect{X: 8, Y: 8, W: 60, H: 30}, &debugdraw.Style{Color: mathx.Color{R: 1, G: 1, A: 1}, Space: debugdraw.Screen})
	d.Text(mathx.Vec2{X: 1, Y: 2.5}, "debug", &debugdraw.Style{Color: mathx.Color{R: 1, G: 1, B: 1, A: 1}})
	d.Circle(ringAt, 1, &debugdraw.Style{Color: mathx.Color{R: 1, B: 1, A: 1}, Thickness: 3})
	return d
}

// drawDebugAt mirrors the module's draw system with an explicit pixel scale.
func drawDebugAt(t *testing.T, s *edit.Session, d *debug.Service, tgt *rt.Target, scale float32) *image.NRGBA {
	t.Helper()
	e, rnd := s.Engine, s.Renderer
	frame := e.GPU().BeginFrame(size, size)
	if err := rnd.BeginFrame(frame, size, size, 1); err != nil {
		t.Fatal(err)
	}
	v := view.View{
		Camera:     view.Camera2D{Position: debugCam, Zoom: 24},
		Viewport:   mathx.Rect{W: size, H: size},
		Target:     tgt.Texture,
		TargetW:    tgt.BucketW,
		TargetH:    tgt.BucketH,
		PixelScale: scale,
		Clear:      render2d.DefaultCamera2D().ClearColor,
		DoClear:    true,
		Layers:     view.AllLayers,
		Label:      "golden.debugdraw",
	}
	f := rnd.View(v)
	fv := f.View()
	px := fv.ScreenToWorld(mathx.Vec2{X: 1}).Sub(fv.ScreenToWorld(mathx.Vec2{})).Len()
	d.Lines(f.Debug(), px, fv.PixelScale, true)
	d.Texts(f, rnd.UIFont(), px)
	f.End()
	rnd.EndFrame()
	e.GPU().EndFrame()
	buf := make([]byte, size*size*4)
	if err := e.GPU().ReadPixels(tgt.Texture, 0, 0, size, size, buf); err != nil {
		t.Fatal(err)
	}
	return &image.NRGBA{Pix: buf, Stride: size * 4, Rect: image.Rect(0, 0, size, size)}
}

func bbox(img *image.NRGBA, match func(r, g, b uint8) bool) image.Rectangle {
	var out image.Rectangle
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			i := img.PixOffset(x, y)
			if match(img.Pix[i], img.Pix[i+1], img.Pix[i+2]) {
				out = out.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return out
}

func yellow(r, g, b uint8) bool { return r > 200 && g > 200 && b < 80 }
func white(r, g, b uint8) bool  { return r > 200 && g > 200 && b > 200 }
func red(r, g, b uint8) bool    { return r > 200 && g < 60 && b < 60 }
func green(r, g, b uint8) bool  { return g > 200 && r < 100 && b < 100 }

func toScreen(p mathx.Vec2) mathx.Vec2 {
	return mathx.Vec2{X: size/2 + (p.X-debugCam.X)*24, Y: size/2 + (p.Y-debugCam.Y)*24}
}

func arrowHead(img *image.NRGBA, scale float32) float32 {
	a, b := toScreen(arrowFrom), toScreen(arrowTo)
	dir := b.Sub(a)
	dir = dir.Scale(1 / dir.Len())
	var head float32
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			i := img.PixOffset(x, y)
			if !green(img.Pix[i], img.Pix[i+1], img.Pix[i+2]) {
				continue
			}
			p := mathx.Vec2{X: float32(x) + 0.5, Y: float32(y) + 0.5}.Sub(b)
			if perp := p.X*dir.Y - p.Y*dir.X; perp > 2.5*scale || perp < -2.5*scale {
				head = max(head, p.Len())
			}
		}
	}
	return head
}

func redRows(img *image.NRGBA, x int) int {
	n := 0
	for y := 0; y < size; y++ {
		i := img.PixOffset(x, y)
		if red(img.Pix[i], img.Pix[i+1], img.Pix[i+2]) {
			n++
		}
	}
	return n
}

func magenta(r, g, b uint8) bool { return r > 200 && b > 200 && g < 80 }

func at(img *image.NRGBA, x, y int, match func(r, g, b uint8) bool) bool {
	if x < 0 || y < 0 || x >= size || y >= size {
		return false
	}
	i := img.PixOffset(x, y)
	return match(img.Pix[i], img.Pix[i+1], img.Pix[i+2])
}

// shaftGaps counts shaft columns whose green pixels are not one vertical run.
func shaftGaps(img *image.NRGBA) (bad, cols int) {
	a, b := toScreen(arrowFrom), toScreen(arrowTo)
	for x := max(int(a.X), 0) + 3; x < int(b.X)-25; x++ {
		if _, solid := greenRun(img, x); solid {
			cols++
		} else {
			cols, bad = cols+1, bad+1
		}
	}
	return bad, cols
}

// ringRun walks a ray out of the ring center and returns the ring width and whether it is solid.
func ringRun(img *image.NRGBA, deg float64) (float32, bool) {
	c := toScreen(ringAt)
	dx, dy := float32(math.Cos(deg*math.Pi/180)), float32(math.Sin(deg*math.Pi/180))
	first, last, misses := float32(-1), float32(-1), 0
	for t := float32(0); t < 60; t += 0.25 {
		hit := at(img, int(c.X+dx*t), int(c.Y-dy*t), magenta)
		switch {
		case hit && first < 0:
			first, last = t, t
		case hit:
			if misses > 0 {
				return last - first, false
			}
			last = t
		case first >= 0:
			misses++
		}
	}
	return last - first, first >= 0
}

func greenRun(img *image.NRGBA, x int) (n int, solid bool) {
	first, last := -1, -1
	for y := 0; y < size; y++ {
		i := img.PixOffset(x, y)
		if green(img.Pix[i], img.Pix[i+1], img.Pix[i+2]) {
			if first < 0 {
				first = y
			}
			last = y
			n++
		}
	}
	return n, n > 0 && last-first+1 == n
}

func TestGameViewDebugDrawPixelScale(t *testing.T) {
	s, pc := playSession(t, size, size, cameraScene)
	pc.Tick(1.0 / 60)
	d := debugPrims(t, pc)
	pool := rt.New(s.Engine.GPU(), gpu.TexSRGBA8, "golden.debugdraw")
	tgt, err := pool.Resize(nil, size, size, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		pool.Release(tgt)
		pool.Close()
	}()
	one := drawDebugAt(t, s, d, tgt, 1)
	compare(t, filepath.Join("testdata", "gameview_debug_draw.png"), one, 8)
	two := drawDebugAt(t, s, d, tgt, 2)
	// a 1 px outline centered on x=16 lands on pixel 15
	if r := bbox(two, yellow); r != image.Rect(15, 15, 136, 76) {
		t.Errorf("screen rect covers %v, want edges at (16,16,120,60) in physical pixels", r)
	}
	h1, h2 := bbox(one, white).Dy(), bbox(two, white).Dy()
	if h1 == 0 || float32(h2)/float32(h1) < 1.75 || float32(h2)/float32(h1) > 2.25 {
		t.Errorf("text height %d at scale 1 and %d at scale 2, want twice", h1, h2)
	}
	if n1, n2 := redRows(one, 40), redRows(two, 40); n1 != 1 || n2 != 1 {
		t.Errorf("line is %d and %d physical pixels wide, want 1", n1, n2)
	}
	a1, a2 := arrowHead(one, 1), arrowHead(two, 2)
	if a1 == 0 || a2/a1 < 1.7 || a2/a1 > 2.3 {
		t.Errorf("arrowhead %.1f px at scale 1 and %.1f px at scale 2, want twice", a1, a2)
	}
	for _, img := range []struct {
		name string
		img  *image.NRGBA
	}{{"x1", one}, {"x2", two}} {
		if bad, cols := shaftGaps(img.img); bad > 0 || cols == 0 {
			t.Errorf("%s: %d of %d shaft columns have gaps", img.name, bad, cols)
		}
		for _, deg := range []float64{0, 45, 90} {
			if w, solid := ringRun(img.img, deg); !solid || w < 1 {
				t.Errorf("%s: ring at %v° is %.2f px wide, solid %v", img.name, deg, w, solid)
			}
		}
	}
	g1, ok1 := greenRun(one, 100)
	g2, ok2 := greenRun(two, 100)
	if !ok1 || !ok2 || g2 < 2*g1-1 || g2 > 2*g1+2 {
		t.Errorf("arrow shaft %d px (solid %v) at scale 1 and %d px (solid %v) at scale 2, want solid and twice", g1, ok1, g2, ok2)
	}
	t.Logf("shaft %d→%d px", g1, g2)
	t.Logf("rect %v, text %d→%d px, arrowhead %.1f→%.1f px", bbox(two, yellow), h1, h2, a1, a2)
	checkGolden(t, "gameview_debug_draw_x2", two, nil)
}
