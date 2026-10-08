package document

import "testing"

func TestPlayViewSwitchesActiveScene(t *testing.T) {
	m := newManager(t)
	d := m.New("scenes/a.aescene", sampleScene(m, 1))
	if err := m.Save(d.ID); err != nil {
		t.Fatal(err)
	}
	ps := sampleScene(m, 2)
	view := d.PlayCopy(ps)
	m.SetPlay(view, nil)
	if !m.InPlay() || m.ActiveScene() != ps || m.Active() != view || m.Edit() != d || m.Get(d.ID) != d {
		t.Fatal("play view must be active while real documents stay reachable")
	}
	a := ps.Find(ps.Root(), "A")
	if err := view.Props.Rename(nil, ps.ID(a), "B"); err != nil {
		t.Fatal(err)
	}
	if view.Dirty() || d.Dirty() || m.Dirty() {
		t.Fatal("play edits must not dirty anything")
	}
	if d.Scene().Find(d.Scene().Root(), "A").IsZero() {
		t.Fatal("edit scene changed")
	}
	m.ClearPlay()
	if m.InPlay() || m.ActiveScene() != d.Scene() || m.Active() != d {
		t.Fatal("stop must return to the edit scene")
	}
}
