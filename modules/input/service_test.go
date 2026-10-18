package input

import (
	"strconv"
	"testing"

	"aego/core/mathx"
	"aego/format/aetext"
	"aego/platform"
	raw "aego/platform/input"
)

type rig struct {
	t    *testing.T
	s    *Service
	in   *raw.State
	now  float64
	tick uint64
}

type counts struct {
	pressed, held, released int
}

func act(name string, k Kind, binds ...string) Action {
	return Action{Name: name, Kind: k, Bindings: mustBindings(binds...)}
}

func one(acts ...Action) Config {
	c := Config{Players: 1, Deadzone: DefaultDeadzone, Stack: []string{"Game"}}
	x := Context{Name: "Game"}
	for _, a := range acts {
		c.Actions = append(c.Actions, a)
		x.Actions = append(x.Actions, a.Name)
	}
	c.Contexts = []Context{x}
	return c
}

func newRig(t *testing.T, c Config) *rig {
	t.Helper()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	return &rig{t: t, s: NewService(c, UserKeys{}), in: &raw.State{}}
}

func (r *rig) key(k raw.Scancode, down bool) {
	kind := platform.EvKeyUp
	if down {
		kind = platform.EvKeyDown
	}
	platform.ApplyInput(r.in, platform.Event{Kind: kind, Key: raw.KeyEvent{Scan: k}})
}

func (r *rig) frame(ticks int, action string) counts {
	r.now += 1.0 / 60
	r.s.Frame(r.in, Clock{Real: r.now, Dt: 1.0 / 60, Game: r.now, Next: r.tick + 1})
	var c counts
	for range ticks {
		r.tick++
		r.s.Tick()
		if action != "" {
			if r.s.Pressed(action) {
				c.pressed++
			}
			if r.s.Held(action) {
				c.held++
			}
			if r.s.Released(action) {
				c.released++
			}
		}
		r.s.EndTick()
	}
	r.in.BeginFrame()
	return c
}

func TestShortPressBetweenTicks(t *testing.T) {
	r := newRig(t, one(act("Jump", Button, "Key.Space")))
	r.key(raw.KeySpace, true)
	r.key(raw.KeySpace, false)
	r.frame(0, "Jump")
	if !r.s.Pressed("Jump") || !r.s.Released("Jump") {
		t.Fatal("frame view must see the tap")
	}
	a, b := r.frame(1, "Jump"), r.frame(1, "Jump")
	if a.pressed+b.pressed != 1 || a.held != 1 || b.released != 1 {
		t.Fatalf("tap delivered as %+v then %+v", a, b)
	}
}

func TestThreeTicksOnePress(t *testing.T) {
	r := newRig(t, one(act("Jump", Button, "Key.Space")))
	r.key(raw.KeySpace, true)
	if c := r.frame(3, "Jump"); c.pressed != 1 || c.held != 3 {
		t.Fatalf("%+v", c)
	}
}

func TestZeroTicksDeliversLater(t *testing.T) {
	r := newRig(t, one(act("Jump", Button, "Key.Space")))
	r.key(raw.KeySpace, true)
	if c := r.frame(0, "Jump"); c.pressed != 0 {
		t.Fatal("no tick ran")
	}
	if c := r.frame(1, "Jump"); c.pressed != 1 {
		t.Fatalf("press was lost: %+v", c)
	}
}

func TestFocusLossReleases(t *testing.T) {
	r := newRig(t, one(act("Jump", Button, "Key.Space")))
	r.key(raw.KeySpace, true)
	r.frame(1, "Jump")
	r.in.Reset()
	if c := r.frame(1, "Jump"); c.released != 1 || c.held != 0 || r.s.Held("Jump") {
		t.Fatalf("%+v", c)
	}
}

func TestPadDisconnectReleases(t *testing.T) {
	r := newRig(t, one(act("Jump", Button, "Pad.A")))
	r.in.ApplyPadAdded(7, "pad")
	r.frame(0, "")
	r.in.ApplyPadButton(7, raw.PadA, true)
	if c := r.frame(1, "Jump"); c.pressed != 1 {
		t.Fatalf("pad press %+v", c)
	}
	r.in.ApplyPadRemoved(7)
	c := r.frame(1, "Jump")
	if c.released != 1 || r.s.Held("Jump") || !r.s.DeviceChanged() {
		t.Fatalf("disconnect %+v changed=%v", c, r.s.DeviceChanged())
	}
}

func menuConfig(consume bool) Config {
	return Config{
		Players: 1, Deadzone: DefaultDeadzone, Stack: []string{"Game", "Menu"},
		Actions: []Action{act("Pause", Button, "Key.Escape"), act("Back", Button, "Key.Escape")},
		Contexts: []Context{
			{Name: "Game", Actions: []string{"Pause"}},
			{Name: "Menu", Priority: 10, Consume: consume, Actions: []string{"Back"}},
		},
	}
}

func TestConsumingContextBlocksLower(t *testing.T) {
	r := newRig(t, menuConfig(true))
	r.key(raw.KeyEscape, true)
	r.frame(0, "")
	if !r.s.Pressed("Back") || r.s.Pressed("Pause") {
		t.Fatal("Menu must consume Escape")
	}
	blocked := ""
	r.s.Each(0, func(i Info) {
		if i.Name == "Pause" {
			blocked = i.Blocked
		}
	})
	if blocked != "Menu" {
		t.Fatalf("blocked by %q", blocked)
	}
	r = newRig(t, menuConfig(false))
	r.key(raw.KeyEscape, true)
	r.frame(0, "")
	if !r.s.Pressed("Back") || !r.s.Pressed("Pause") {
		t.Fatal("without consume both must fire")
	}
}

func TestContextSwitchSendsNoFakeEvents(t *testing.T) {
	c := one(act("Jump", Button, "Key.Space"))
	c.Contexts = append(c.Contexts, Context{Name: "Menu"})
	r := newRig(t, c)
	r.key(raw.KeySpace, true)
	r.frame(1, "Jump")
	r.s.SetContext("Menu")
	if x := r.frame(1, "Jump"); x.released != 0 {
		t.Fatal("leaving a context sent Released")
	}
	r.s.SetContext("Game")
	if x := r.frame(1, "Jump"); x.pressed != 0 || x.held != 1 {
		t.Fatalf("entering a context: %+v", x)
	}
	c.Contexts[0].Release = true
	r = newRig(t, c)
	r.key(raw.KeySpace, true)
	r.frame(1, "Jump")
	r.s.SetContext("Menu")
	if x := r.frame(1, "Jump"); x.released != 1 {
		t.Fatal("release=true must send Released")
	}
}

func TestCompositeIsNormalized(t *testing.T) {
	r := newRig(t, one(act("Move", Axis2D, "Key.W/Key.S/Key.A/Key.D")))
	r.key(raw.KeyW, true)
	r.key(raw.KeyD, true)
	r.frame(0, "")
	v := r.s.Axis2("Move")
	if mathx.Abs(v.Len()-1) > 1e-5 || v.X <= 0 || v.Y >= 0 {
		t.Fatalf("WASD gave %v", v)
	}
}

func TestDeadzoneInvertSmoothing(t *testing.T) {
	sm := act("Smooth", Axis1D, "Key.A/Key.D")
	sm.Smoothing = 0.1
	r := newRig(t, one(act("X", Axis1D, "Pad.LeftX deadzone=0.2"), act("Inv", Axis1D, "Pad.LeftX invert deadzone=0.2"), sm))
	r.in.ApplyPadAdded(1, "pad")
	r.in.ApplyPadAxis(1, raw.PadLeftX, 0.1)
	r.frame(0, "")
	if r.s.Axis("X") != 0 {
		t.Fatal("inside the dead zone")
	}
	r.in.ApplyPadAxis(1, raw.PadLeftX, 0.6)
	r.frame(0, "")
	if x := r.s.Axis("X"); mathx.Abs(x-0.5) > 1e-5 || mathx.Abs(r.s.Axis("Inv")+0.5) > 1e-5 {
		t.Fatalf("rescale %v inv %v", x, r.s.Axis("Inv"))
	}
	r.key(raw.KeyD, true)
	r.frame(0, "")
	if v := r.s.Axis("Smooth"); v <= 0 || v >= 1 {
		t.Fatalf("smoothing first frame %v", v)
	}
	for range 120 {
		r.frame(0, "")
	}
	if v := r.s.Axis("Smooth"); v < 0.99 {
		t.Fatalf("smoothing settles at %v", v)
	}
}

func TestInteractions(t *testing.T) {
	hold := act("Hold", Button, "Key.H")
	hold.Interaction = Interaction{Trigger: Hold, Time: 0.1}
	tap := act("Tap", Button, "Key.T")
	tap.Interaction = Interaction{Trigger: Tap}
	dbl := act("Dbl", Button, "Key.B")
	dbl.Interaction = Interaction{Trigger: DoubleTap}
	tog := act("Tog", Button, "Key.G")
	tog.Interaction = Interaction{Trigger: Toggle}
	r := newRig(t, one(hold, tap, dbl, tog, act("Save", Button, "Key.Z+Key.S")))

	r.key(raw.KeyH, true)
	presses := 0
	for i := range 10 {
		r.frame(0, "")
		if r.s.Pressed("Hold") {
			presses++
			if i < 5 {
				t.Fatal("hold fired early")
			}
		}
	}
	if presses != 1 || !r.s.Held("Hold") {
		t.Fatalf("hold presses %d", presses)
	}

	r.key(raw.KeyT, true)
	r.frame(0, "")
	r.key(raw.KeyT, false)
	r.frame(0, "")
	if !r.s.Pressed("Tap") {
		t.Fatal("tap")
	}

	for range 2 {
		r.key(raw.KeyB, true)
		r.frame(0, "")
		r.key(raw.KeyB, false)
		r.frame(0, "")
	}
	r.key(raw.KeyB, true)
	r.frame(0, "")
	r.key(raw.KeyB, false)
	r.frame(0, "")

	r.key(raw.KeyG, true)
	r.frame(0, "")
	r.key(raw.KeyG, false)
	r.frame(0, "")
	if !r.s.Held("Tog") {
		t.Fatal("toggle on")
	}
	r.key(raw.KeyG, true)
	r.frame(0, "")
	if r.s.Held("Tog") || !r.s.Released("Tog") {
		t.Fatal("toggle off")
	}

	r.key(raw.KeyS, true)
	r.frame(0, "")
	if r.s.Pressed("Save") {
		t.Fatal("chord without the modifier")
	}
	r.key(raw.KeyS, false)
	r.key(raw.KeyZ, true)
	r.frame(0, "")
	r.key(raw.KeyS, true)
	r.frame(0, "")
	if !r.s.Pressed("Save") {
		t.Fatal("chord")
	}
}

func TestDoubleTapFiresOnSecondPress(t *testing.T) {
	dbl := act("Dbl", Button, "Key.B")
	dbl.Interaction = Interaction{Trigger: DoubleTap}
	r := newRig(t, one(dbl))
	fired := 0
	for range 2 {
		r.key(raw.KeyB, true)
		r.frame(0, "")
		if r.s.Pressed("Dbl") {
			fired++
		}
		r.key(raw.KeyB, false)
		r.frame(0, "")
	}
	if fired != 1 {
		t.Fatalf("double tap fired %d times", fired)
	}
}

func TestBufferAndConsume(t *testing.T) {
	j := act("Jump", Button, "Key.Space")
	j.Buffer = 0.1
	r := newRig(t, one(j))
	r.key(raw.KeySpace, true)
	r.frame(0, "")
	r.key(raw.KeySpace, false)
	for range 3 {
		r.frame(0, "")
	}
	if !r.s.Buffered("Jump", 0) {
		t.Fatal("press must stay buffered for 0.1 s")
	}
	r.s.Consume("Jump")
	if r.s.Buffered("Jump", 0) {
		t.Fatal("consume must clear the buffer")
	}
	r.key(raw.KeySpace, true)
	r.frame(0, "")
	r.key(raw.KeySpace, false)
	for range 10 {
		r.frame(0, "")
	}
	if r.s.Buffered("Jump", 0) {
		t.Fatal("buffer outlived its window")
	}
}

func TestTwoPlayersTwoDevices(t *testing.T) {
	c := one(act("Jump", Button, "Key.Space", "Pad.A"))
	c.Players = 2
	r := newRig(t, c)
	r.in.ApplyPadAdded(3, "pad")
	r.frame(0, "")
	r.in.ApplyPadButton(3, raw.PadA, true)
	r.frame(0, "")
	if !r.s.PressedP(1, "Jump") || r.s.PressedP(0, "Jump") || r.s.ActiveDevice(1) != 1 {
		t.Fatal("pad must go to player 1")
	}
	r.key(raw.KeySpace, true)
	r.frame(0, "")
	if !r.s.PressedP(0, "Jump") || r.s.PressedP(1, "Jump") {
		t.Fatal("keyboard belongs to player 0")
	}
}

func TestRebindConflictAndReset(t *testing.T) {
	r := newRig(t, one(act("Jump", Button, "Key.Space"), act("Fire", Button, "Key.K")))
	r.s.BeginRebind("Jump", 0)
	r.frame(0, "")
	if !r.s.Rebinding() {
		t.Fatal("rebind ended without input")
	}
	r.key(raw.KeyK, true)
	r.frame(0, "")
	if r.s.Rebinding() || r.s.RebindConflict() != "Fire" || r.s.Glyph("Jump") != "[K]" {
		t.Fatalf("rebind %v %q %q", r.s.Rebinding(), r.s.RebindConflict(), r.s.Glyph("Jump"))
	}
	r.s.ResetBinding("Jump")
	if r.s.Glyph("Jump") != "[Space]" {
		t.Fatalf("reset gave %q", r.s.Glyph("Jump"))
	}
}

func TestMigrateAxes(t *testing.T) {
	src := "input {\n  axes {\n    horizontal {\n      negative=A\n      positive=D\n      altNegative=Left\n      altPositive=Right\n    }\n  }\n}\n"
	n, err := aetext.Parse([]byte(src), "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(n, 1, Version); err != nil {
		t.Fatal(err)
	}
	var c Config
	if err := Decode(n, &c); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if !c.Migrated || len(c.Actions) != 1 || c.Actions[0].Kind != Axis1D || c.Actions[0].Bindings[1].String() != "Key.Left/Key.Right" {
		t.Fatalf("%+v", c)
	}
}

func TestSectionRoundTrip(t *testing.T) {
	c := one(act("Move", Axis2D, "Key.W/Key.S/Key.A/Key.D", "Pad.LeftStick deadzone=0.25"), act("Jump", Button, "Key.Space", "Pad.A"))
	c.Actions[1].Buffer = 0.1
	out := &aetext.Node{Head: ID}
	Encode(&c, out)
	text := aetext.Write(out)
	n, err := aetext.Parse(text, "test")
	if err != nil {
		t.Fatal(err)
	}
	var back Config
	if err := Decode(n, &back); err != nil {
		t.Fatal(err)
	}
	again := &aetext.Node{Head: ID}
	Encode(&back, again)
	if string(aetext.Write(again)) != string(text) {
		t.Fatalf("round trip differs:\n%s\n---\n%s", text, aetext.Write(again))
	}
}

func TestFrameIsAllocFree(t *testing.T) {
	var acts []Action
	for i := range 32 {
		acts = append(acts, act("a"+strconv.Itoa(i), Button, "Key.Space", "Pad.A"))
	}
	r := newRig(t, one(acts...))
	r.in.ApplyPadAdded(1, "a")
	r.in.ApplyPadAdded(2, "b")
	r.in.ApplyPadAxis(1, raw.PadLeftX, 0.7)
	r.frame(1, "")
	n := testing.AllocsPerRun(200, func() {
		r.now += 1.0 / 60
		r.s.Frame(r.in, Clock{Real: r.now, Dt: 1.0 / 60, Game: r.now, Next: r.tick + 1})
		r.s.Tick()
		_ = r.s.Pressed("a7")
		r.s.EndTick()
	})
	if n != 0 {
		t.Fatalf("%v allocations per frame", n)
	}
}

func TestFocusInfoNamesTheBlocker(t *testing.T) {
	r := newRig(t, menuConfig(true))
	r.key(raw.KeyEscape, true)
	r.frame(0, "")
	var info Info
	r.s.Each(0, func(i Info) {
		if i.Name == "Pause" {
			info = i
		}
	})
	if info.Context != "Game" || info.Blocked != "Menu" || info.Why != "Key.Escape" {
		t.Fatalf("%+v", info)
	}
	if st := r.s.Stack(); len(st) != 2 || st[0].Name != "Menu" || !st[0].Consume {
		t.Fatalf("stack %+v", st)
	}
}

func TestInjectTickZeroAppliesNextFrame(t *testing.T) {
	r := newRig(t, one(act("Jump", Button, "Key.Space")))
	r.frame(0, "")
	if err := r.s.Inject(0, "Jump", 1); err != nil {
		t.Fatal(err)
	}
	r.frame(0, "")
	if !r.s.Pressed("Jump") {
		t.Fatal("tick 0 must apply on the next frame")
	}
}
