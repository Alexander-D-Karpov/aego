//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/platform/clip"
	"aego/platform/input"
	"aego/platform/sdl"
	"aego/ui/draw"
)

// Drive the real clipboard, SDL keyboard/mouse events, dock manager and
// presented frame. The documentation backend cannot send a conversation.
func TestDiagnosticComposerConsoleProfilerNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const width, height = 1280, 900
	p := platformerAuthoringProject(t)
	logger := log.New()
	s, err := Open(p, Config{Width: width, Height: height, Hidden: true, NoWatch: true, Logger: logger, AgentBackend: screenshotAgent{}}, platformerAuthoringModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	step := func(n int) {
		t.Helper()
		for range n {
			if !s.Step() {
				t.Fatal("editor stopped")
			}
		}
	}
	step(1)
	ed, ws := s.App.Context(), s.App.Workspace()
	if _, err = ed.Docs.Open("scenes/main.aescene"); err != nil {
		t.Fatal(err)
	}
	fullscreen := func(panel string) {
		ws.Show(panel)
		ws.Focus(panel)
		if ws.Tree.Maximized() != panel {
			if !ws.ToggleMaximize(panel) {
				t.Fatal("could not maximize", panel)
			}
		}
		step(3)
	}
	screenshot := func(name string) *image.RGBA {
		t.Helper()
		var im *image.RGBA
		s.Engine.CaptureFrame(func(got *image.RGBA, e error) {
			if e != nil {
				t.Fatal(e)
			}
			im = got
		})
		step(1)
		if im == nil || im.Bounds().Dx() != width || im.Bounds().Dy() != height {
			t.Fatal("missing native frame", name)
		}
		if dir := os.Getenv("AEGO_EDITOR_WORKFLOW_SHOTS"); dir != "" {
			if e := os.MkdirAll(dir, 0755); e != nil {
				t.Fatal(e)
			}
			f, e := os.Create(filepath.Join(dir, name+".png"))
			if e != nil {
				t.Fatal(e)
			}
			e = png.Encode(f, im)
			ce := f.Close()
			if e != nil || ce != nil {
				t.Fatal(e, ce)
			}
		}
		return im
	}
	screen := mathx.Rect{W: width, H: height}
	drawn := func(prefix string) []draw.TestDrawnItem {
		return s.App.UIList().TestDrawnItems(screen, ed.UI.Font(), prefix, 0, 0)
	}
	text := func(prefix string) draw.TestDrawnItem {
		t.Helper()
		items := drawn(prefix)
		for _, it := range items {
			if it.Clip.Contains(it.Bounds.Center()) {
				return it
			}
		}
		t.Fatalf("%q not visible: %+v", prefix, items)
		return draw.TestDrawnItem{}
	}
	inside := func(it draw.TestDrawnItem) bool {
		r, c := it.Bounds, it.Clip
		return r.W > 0 && r.H > 0 && r.X >= c.X-1 && r.Y >= c.Y-1 && r.X+r.W <= c.X+c.W+1 && r.Y+r.H <= c.Y+c.H+1
	}
	mouse := func(at mathx.Vec2, right bool) {
		t.Helper()
		for i := range 3 {
			fn := sdl.TestPointerEvent
			if right {
				fn = sdl.TestRightPointerEvent
			}
			if !fn(at.X, at.Y, i) {
				t.Fatal("SDL mouse rejected")
			}
			step(1)
		}
		step(1)
	}
	key := func(k input.Scancode, ctrl, shift bool) {
		t.Helper()
		if !sdl.TestEditKey(k, true, ctrl, shift) {
			t.Fatal("SDL rejected key")
		}
		step(1)
		in := s.Engine.Input()
		if !in.KeyPressed(k) || in.Mods().Has(input.ModCtrl) != ctrl || in.Mods().Has(input.ModShift) != shift {
			t.Fatalf("SDL did not deliver key press %v with modifiers ctrl=%v shift=%v (pressed=%v mods=%v)", k, ctrl, shift, in.KeyPressed(k), in.Mods())
		}
		if !sdl.TestEditKey(k, false, false, false) {
			t.Fatal("SDL rejected key release")
		}
		step(1)
		if !in.KeyReleased(k) {
			t.Fatalf("SDL did not deliver key release %v", k)
		}
		step(1)
	}
	fullscreen(workspace.Scene)
	source := screenshot("01-source")
	var original bytes.Buffer
	if err = png.Encode(&original, source); err != nil {
		t.Fatal(err)
	}
	fullscreen("agent")
	panel := s.App.Agent()
	panel.Ask("draft line")
	step(3)
	small := text("draft line").Clip
	if !ed.Clipboard.SetData(clip.MimePNG, original.Bytes(), "") {
		t.Fatal("SDL clipboard rejected window screenshot")
	}
	key(input.KeyV, true, false)
	if panel.Input() != "draft line" {
		t.Fatalf("screenshot paste changed text: %q", panel.Input())
	}
	key(input.KeyA, true, false)
	long := "line 01\nline 02\nline 03\nline 04\nline 05\nline 06\nline 07\nline 08\nlast visible line"
	ed.Clipboard.SetText(long)
	key(input.KeyV, true, false)
	if panel.Input() != long {
		t.Fatalf("text not editable after clipboard image: %q", panel.Input())
	}
	last := text("last visible line")
	if !inside(last) || last.Clip.H < small.H+3*ed.UI.LineHeight() {
		t.Fatalf("multiline did not grow: one=%+v multiline=%+v", small, last)
	}
	send := text("Send")
	if send.Bounds.Center().Y < last.Clip.Y+last.Clip.H-ed.UI.Theme.RowHeight {
		t.Fatalf("Send is not aligned to the bottom of the multiline draft: %+v", send)
	}
	mouse(mathx.V2(send.Bounds.Center().X, last.Clip.Y+7), false)
	if panel.Input() != long {
		t.Fatal("Click above Send submitted the draft")
	}
	panel.Ask("")
	step(2)
	key(input.KeyEnter, false, true)
	if panel.Input() != long+"\n" {
		t.Fatalf("shift-enter lost draft or sent text: %q", panel.Input())
	}
	thumbs := s.App.UIList().TestDrawnItems(screen, ed.UI.Font(), "", 512, height*512/width)
	if len(thumbs) != 1 || !inside(thumbs[0]) || thumbs[0].Bounds.Y+thumbs[0].Bounds.H > last.Clip.Y+1 {
		t.Fatalf("attached screenshot overlaps draft, clipped or missing: %+v; draft %+v", thumbs, last)
	}
	imageFrame := screenshot("02-image-multiline")
	tile := thumbs[0].Bounds
	colors := map[uint32]bool{}
	for y := int(tile.Y) + 3; y < int(tile.Y+tile.H)-3; y++ {
		for x := int(tile.X) + 3; x < int(tile.X+tile.W)-22; x++ {
			c := imageFrame.RGBAAt(x, y)
			colors[uint32(c.R)<<16|uint32(c.G)<<8|uint32(c.B)] = true
		}
	}
	if len(colors) < 16 {
		t.Fatal("pasted screenshot has no visible thumbnail", len(colors))
	}
	t.Logf("single/multiline=%.0f/%.0f, PNG bytes %d; thumbnail bounds=%+v colors=%d", small.H, last.Clip.H, original.Len(), tile, len(colors))

	fullscreen(workspace.Console)
	const id = "45e397813812442d994a810000000001"
	const raw = "GL33 link/layout variant default: MAT004: postfx shader cannot draw sprite geometry"
	logger.Log(log.Render, log.Error, "shader candidate failed", log.F("code", "SHD009"), log.F("file", "assets/postfx/cold"), log.F("line", 0), log.F("err", raw), log.F("asset", id))
	step(3)
	openMenu := func() {
		t.Helper()
		row := text("shader candidate failed")
		// Open near the right edge of the row to exercise the popup's screen clamp.
		mouse(mathx.V2(min(row.Clip.X+row.Clip.W-10, width-12), row.Bounds.Center().Y), true)
		for _, item := range []string{"Copy message", "Copy full details", "Copy location", "Investigate with Agent"} {
			if !inside(text(item)) {
				t.Fatal("popup action clipped", item)
			}
		}
	}
	openMenu()
	screenshot("03-console-menu")
	mouse(text("Copy full details").Bounds.Center(), false)
	details := ed.Clipboard.Text()
	var msg services.ConsoleMessage
	if json.Unmarshal([]byte(details), &msg) != nil || msg.Code != "SHD009" || msg.Cause != "MAT004" || msg.Line != 0 || msg.Asset != id || msg.Path != "assets/postfx/cold" || msg.Level != "ERROR" || msg.Channel != "render" || !strings.Contains(msg.Text, raw) {
		t.Fatalf("copy action selected wrong diagnostic: %s", details)
	}
	openMenu()
	mouse(text("Investigate with Agent").Bounds.Center(), false)
	draft := panel.Input()
	if !strings.HasPrefix(draft, long+"\n\n\n") || !strings.HasSuffix(draft, details) || !strings.Contains(draft, "Source line unknown.") {
		t.Fatalf("draft lost selected console metadata: %q", draft)
	}
	if names, e := agentui.List(p.Root); e != nil || len(names) != 0 {
		t.Fatal("investigating entry unexpectedly sent a request", names, e)
	}
	key(input.KeyEnter, false, true)
	if !strings.Contains(panel.Input(), details) || panel.Input() == draft {
		t.Fatalf("Console draft is not editable, got %q", panel.Input())
	}
	narrow := text("}").Clip
	narrowSend := text("Send")
	if narrow.W >= 300 || !inside(narrowSend) || narrowSend.Bounds.Y < narrow.Y+narrow.H || narrowSend.Bounds.Center().X < narrow.X || narrowSend.Bounds.Center().X > narrow.X+narrow.W {
		t.Fatalf("narrow composer lost wrap width: field=%+v button=%+v", narrow, narrowSend)
	}
	t.Logf("docked composer width %.0f px, action below the draft", narrow.W)
	screenshot("04-editable-diagnostic")
	fullscreen("agent")
	if images := s.App.UIList().TestDrawnItems(screen, ed.UI.Font(), "", 512, height*512/width); len(images) != 1 || !inside(images[0]) {
		t.Fatal("adding a diagnostic dropped the unsent attachment")
	}
	screenshot("04b-diagnostic-maximized")
	b, e := s.agentBridge()
	if e != nil {
		t.Fatal(e)
	}
	cfg := b.Backend()
	// The Console popup and the MCP resource must return the exact same record,
	// including its timestamp and unknown source line. The client stays local.
	request, e := http.NewRequest(http.MethodPost, cfg.MCPURL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"aego://editor/console"}}`))
	if e != nil {
		t.Fatal(e)
	}
	request.Header.Set("Authorization", "Bearer "+cfg.Token)
	request.Header.Set("Content-Type", "application/json")
	done := make(chan struct{})
	var result struct {
		Result struct{ Contents []struct{ URI, Text string } }
	}
	var readErr error
	go func() {
		defer close(done)
		r, e := (&http.Client{Timeout: 10 * time.Second}).Do(request)
		if e != nil {
			readErr = e
			return
		}
		defer r.Body.Close()
		readErr = json.NewDecoder(r.Body).Decode(&result)
	}()
	for {
		select {
		case <-done:
			goto received
		default:
			step(1)
		}
	}
received:
	if readErr != nil || len(result.Result.Contents) != 1 || result.Result.Contents[0].URI != "aego://editor/console" {
		t.Fatal("editor Console resource", result, readErr)
	}
	var logItems []services.ConsoleMessage
	if err = json.Unmarshal([]byte(result.Result.Contents[0].Text), &logItems); err != nil || !slices.Contains(logItems, msg) {
		t.Fatalf("resource did not expose selected Console entry: %v %v", logItems, err)
	}
	t.Logf("aego://editor/console returned the same selected %s / %s record in %d entries", msg.Code, msg.Cause, len(logItems))
	fullscreen(workspace.Profiler)
	runtime.GC()
	step(3)
	for range 10 {
		items := drawn("Live at last GC")
		if len(items) > 0 && inside(items[0]) {
			break
		}
		if !sdl.TestPointerEvent(400, 400, 0) || !sdl.TestWheelEvent(400, 400, -2) {
			t.Fatal("SDL rejected wheel")
		}
		step(2)
	}
	for _, prefix := range []string{"Live at last GC", "Go mapped"} {
		it := text(prefix)
		if !inside(it) || !strings.Contains(it.Text, "MiB") {
			t.Fatalf("memory readout clipped: %+v", it)
		}
		t.Log(it.Text)
	}
	screenshot("05-profiler-memory")
}
