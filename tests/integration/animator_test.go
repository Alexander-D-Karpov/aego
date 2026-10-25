package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/anim"
	"aego/app/agent"
	"aego/app/edit"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/panels/animator"
	"aego/platform"
	"aego/platform/input"
	"aego/project"
	"aego/sdk"
)

const playerCtrlRel = "assets/anim/player.aectrl"

func animatorProject(t *testing.T) (*edit.Session, *project.Project) {
	t.Helper()
	pr, err := project.Load(copyExample(t, "platformer"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(pr, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Step()
	return s, pr
}

func animatorSession(t *testing.T, s *edit.Session) *animator.Panel {
	t.Helper()
	ctx := s.App.Context()
	sc := ctx.Scene()
	ctx.Select(sc.ID(sc.Find(sc.Root(), "Player")))
	s.App.Workspace().Show(animator.ID)
	for range 5 {
		s.Step()
	}
	p := s.App.AnimatorPanel()
	if p.Graph() == nil {
		t.Fatal("the Animator panel did not open the Player's controller")
	}
	return p
}

func ctrlBytes(t *testing.T, pr *project.Project) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(pr.Root, playerCtrlRel))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// Graph edits land in the controller file one History step each, and Undo puts back the original bytes.
func TestAnimatorEditsUndoToTheOriginalBytes(t *testing.T) {
	s, pr := animatorProject(t)
	p := animatorSession(t, s)
	before := ctrlBytes(t, pr)
	g := p.Graph()
	run := g.StateIndex("Run")
	idle := g.StateIndex("Idle")
	edits := []func() error{
		func() error { return p.MoveState(int(idle), [2]float32{12, 34}) },
		func() error { return p.MoveState(-1, [2]float32{-200, 0}) },
		func() error { return p.MoveState(-2, [2]float32{-200, 120}) },
		func() error { return p.AddState([2]float32{400, 300}) },
		func() error { return p.AddTransition(int(run), len(p.Graph().States)-1) },
		func() error { return p.MoveTransition(1, -1) },
		func() error { return p.SetCond(0, 0, anim.Cond{Param: 0, Op: anim.OpGe, Value: 2}) },
		func() error { return p.AddParam(anim.ParamTrigger) },
		func() error { return p.AutoLayout() },
		func() error { return p.RemoveState(int(run)) },
	}
	d := s.App.Context().Doc()
	for i, e := range edits {
		if err := e(); err != nil {
			t.Fatalf("edit %d: %v", i, err)
		}
		s.Step()
	}
	after := ctrlBytes(t, pr)
	if bytes.Equal(after, before) || !strings.Contains(string(after), "entryPos=") {
		t.Fatalf("the edits did not reach the file:\n%s", after)
	}
	for range edits {
		if err := d.History.Undo(); err != nil {
			t.Fatal(err)
		}
	}
	s.Step()
	if got := ctrlBytes(t, pr); !bytes.Equal(got, before) {
		t.Fatalf("Undo did not restore the controller:\n%s", got)
	}
}

// The agent writes a state's pos and the open panel shows it.
func TestAnimatorRereadsAnAgentEdit(t *testing.T) {
	s, pr := animatorProject(t)
	p := animatorSession(t, s)
	sess, err := agent.Open(pr, agent.Options{Mods: []sdk.Module{combatModule{}}})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if _, err := agent.Run(sess, "ctrl.set-state", agent.Args{"file": playerCtrlRel, "name": "Idle", "attr": "pos", "value": "10,20"}); err != nil {
		t.Fatal(err)
	}
	id, ok := s.App.Context().Pipeline.Lookup(playerCtrlRel)
	if !ok {
		t.Fatal("no asset at " + playerCtrlRel)
	}
	if _, err := s.App.Context().Pipeline.Reimport(id); err != nil {
		t.Fatal(err)
	}
	s.Step()
	g := p.Graph()
	if st := g.States[g.StateIndex("Idle")]; st.Pos != [2]float32{10, 20} {
		t.Fatalf("the panel did not reread the controller: Idle at %v", st.Pos)
	}
}

func clickAt(t *testing.T, s *edit.Session, at mathx.Vec2, b input.Button) {
	t.Helper()
	push(t, s,
		platform.Event{Kind: platform.EvMouseMove, Mouse: input.MouseEvent{Pos: at}},
		platform.Event{Kind: platform.EvMouseDown, Mouse: input.MouseEvent{Pos: at, Button: b}},
		platform.Event{Kind: platform.EvMouseUp, Mouse: input.MouseEvent{Pos: at, Button: b}},
	)
	s.Step()
}

// The node menu, the transition menu and the clip picker stay open after the click that opens them.
func TestAnimatorMenusOpenWithTheMouse(t *testing.T) {
	s := workspaceSession(t)
	p := animatorSession(t, s)
	idle := int(p.Graph().StateIndex("Idle"))
	r, _ := p.NodeRect(idle)
	clickAt(t, s, mathx.Vec2{X: r.X + 20, Y: r.Y + r.H/2}, input.MouseRight)
	if !p.MenuOpen() {
		t.Fatal("the state menu did not stay open")
	}
	if kind, i := p.Selected(); kind != "state" || i != idle {
		t.Fatalf("a right click selected %s %d", kind, i)
	}
	clickAt(t, s, mathx.Vec2{X: 2, Y: 2}, input.MouseLeft)
	s.Step()
	at, ok := p.EdgePoint(0)
	if !ok {
		t.Fatal("transition 0 is not drawn")
	}
	clickAt(t, s, at, input.MouseRight)
	if kind, _ := p.Selected(); kind != "transition" || !p.MenuOpen() {
		t.Fatalf("the transition menu did not stay open (selected %s)", kind)
	}
	clickAt(t, s, mathx.Vec2{X: 2, Y: 2}, input.MouseLeft)
	p.SelectState(idle)
	for range 3 {
		s.Step()
	}
	cr := p.ClipPickerRect()
	if cr.W == 0 {
		t.Fatal("the state inspector has no clip picker")
	}
	clickAt(t, s, mathx.Vec2{X: cr.X + cr.W/2, Y: cr.Y + cr.H/2}, input.MouseLeft)
	found := p.ClipPickerOpen()
	if !found {
		t.Fatal("the clip picker did not stay open after a click")
	}
}

func TestAnimatorPanelFrameAllocs(t *testing.T) {
	skipUnderRace(t)
	s := workspaceSession(t)
	ctx := s.App.Context()
	sc := ctx.Scene()
	s.App.Workspace().Show(animator.ID)
	measure := func(node string) float64 {
		ctx.Select(sc.ID(sc.Find(sc.Root(), node)))
		for range 60 {
			s.Step()
		}
		return testing.AllocsPerRun(30, func() { s.Step() })
	}
	empty := measure("Camera")
	graph := measure("Player")
	if s.App.AnimatorPanel().Graph() == nil {
		t.Fatal("the panel did not open the Player's controller")
	}
	s.App.AnimatorPanel().SelectTransition(0)
	for range 5 {
		s.Step()
	}
	inspector := testing.AllocsPerRun(30, func() { s.Step() })
	if graph > empty || inspector > empty {
		t.Fatalf("frame allocations: empty panel %v, graph %v, transition inspector %v; the graph must not allocate", empty, graph, inspector)
	}
}
