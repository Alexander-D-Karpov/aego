package edit

import (
	"testing"

	"aego/scene"
	"aego/sdk"
)

type step struct {
	kind  string
	scene *scene.Scene
}

type tracker struct {
	log *[]step
}

func (tracker) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "track"}
}

func (tracker) Register(*sdk.Registry) error {
	return nil
}

func (t tracker) AttachWorld(ctx *sdk.AttachContext) error {
	*t.log = append(*t.log, step{"attach", ctx.Scene})
	return nil
}

func (t tracker) DetachWorld(ctx *sdk.AttachContext) {
	*t.log = append(*t.log, step{"detach", ctx.Scene})
}

func TestModulesAttachPerDocument(t *testing.T) {
	p := newProject(t)
	writeScene(t, p, "scenes/a.aescene", func(s *scene.Scene) { s.Create(s.Root(), "A") })
	writeScene(t, p, "scenes/b.aescene", func(s *scene.Scene) { s.Create(s.Root(), "B") })
	var log []step
	s, err := Open(p, headless(), tracker{&log})
	if err != nil {
		t.Fatal(err)
	}
	s.Step()
	ctx := s.App.Context()
	da, err := ctx.Docs.Open("scenes/a.aescene")
	if err != nil {
		t.Fatal(err)
	}
	db, err := ctx.Docs.Open("scenes/b.aescene")
	if err != nil {
		t.Fatal(err)
	}
	s.Step()
	pc := s.App.Play()
	if err := pc.Start(da); err != nil {
		t.Fatal(err)
	}
	play := pc.Scene()
	s.Step()
	pc.Stop()
	s.Step()
	ea, eb := da.Scene(), db.Scene()
	s.Close()
	want := []step{{"attach", ea}, {"attach", eb}, {"attach", play}, {"detach", play}, {"detach", eb}, {"detach", ea}}
	var got []step
	for _, st := range log {
		if st.scene == ea || st.scene == eb || st.scene == play {
			got = append(got, st)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("events = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d = %v, want %v", i, got[i], want[i])
		}
	}
}
