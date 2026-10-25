package animedit_test

import (
	"bytes"
	"strings"
	"testing"

	"aego/core/curve"
	"aego/core/mathx"
	"aego/editor/core/animedit"
	"aego/schema"
)

func (r *rig) trackOf(prop string) int {
	r.t.Helper()
	for i, t := range r.svc.Clip().Tracks {
		if t.Property == prop {
			return i
		}
	}
	r.t.Fatalf("no track %s", prop)
	return -1
}

func TestCurveEditKeepsOtherLanes(t *testing.T) {
	r := newRig(t)
	orig := bytes.Clone(r.files[clipRel])
	tr := r.trackOf("transform.position")
	if r.svc.Lanes(tr) != 2 {
		t.Fatalf("lanes %d", r.svc.Lanes(tr))
	}
	before := r.svc.LaneCurve(tr, 1)
	after := before
	after.Keys = append([]curve.Key(nil), before.Keys...)
	after.Keys[1].V = 3
	after.Keys[1].Tangent, after.Keys[1].In, after.Keys[1].Out = curve.TangentFree, 0.5, 0.25
	if err := r.svc.EditClip("Curve", r.svc.CurveOps(tr, 1, before, after)...); err != nil {
		t.Fatal(err)
	}
	k := r.svc.Clip().Tracks[tr].Keys[1]
	if k.V.L[0] != 2 || k.V.L[1] != 3 || k.Tangent != curve.TangentFree || k.In[1] != 0.5 || k.Out[1] != 0.25 {
		t.Fatalf("key %+v: x must stay 2, y becomes 3 with free tangents", k)
	}
	before = r.svc.LaneCurve(tr, 0)
	after = before
	after.Keys = append([]curve.Key(nil), before.Keys...)
	after.Keys[1].T = 0.75
	if err := r.svc.EditClip("Move", r.svc.CurveOps(tr, 0, before, after)...); err != nil {
		t.Fatal(err)
	}
	k = r.svc.Clip().Tracks[tr].Keys[1]
	if !near(k.T, 0.75) || k.V.L[1] != 3 {
		t.Fatalf("moved key %+v", k)
	}
	before = r.svc.LaneCurve(tr, 0)
	after = curve.Curve{Keys: before.Keys[:1]}
	if err := r.svc.EditClip("Remove", r.svc.CurveOps(tr, 0, before, after)...); err != nil {
		t.Fatal(err)
	}
	if n := len(r.svc.Clip().Tracks[tr].Keys); n != 1 {
		t.Fatalf("%d keys after removing one", n)
	}
	for range 3 {
		if err := r.d.History.Undo(); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(r.files[clipRel], orig) {
		t.Fatalf("undo must restore the clip bytes:\n%s", r.files[clipRel])
	}
}

func TestCurveAddSamplesOtherLanes(t *testing.T) {
	r := newRig(t)
	tr := r.trackOf("SpriteRenderer.tint")
	before := r.svc.LaneCurve(tr, 0)
	after := before
	after.Keys = []curve.Key{before.Keys[0], {T: 0.5, V: 0.9}, before.Keys[1]}
	if err := r.svc.EditClip("Add", r.svc.CurveOps(tr, 0, before, after)...); err != nil {
		t.Fatal(err)
	}
	k := r.svc.Clip().Tracks[tr].Keys[1]
	if !near(k.V.L[0], 0.9) || !near(k.V.L[2], 0.5) || !near(k.V.L[3], 1) {
		t.Fatalf("new key %+v: red 0.9, blue sampled 0.5", k.V)
	}
}

func TestPasteIntoAnotherClipCreatesTheTrack(t *testing.T) {
	r := newRig(t)
	r.files["assets/anim/other.aeanim"] = []byte("anim version=1 length=2 loop=once {}\n")
	tr := r.trackOf("SpriteRenderer.tint")
	if n := r.svc.CopyKeys([]animedit.KeyRef{{Track: tr, T: 0}, {Track: tr, T: 1}}); n != 2 {
		t.Fatalf("copied %d", n)
	}
	if err := r.svc.SetClip("assets/anim/other.aeanim"); err != nil {
		t.Fatal(err)
	}
	r.svc.SetMode(animedit.Preview)
	r.svc.SetTime(0.5)
	if err := r.svc.PasteKeys(); err != nil {
		t.Fatal(err)
	}
	c := r.svc.Clip()
	if len(c.Tracks) != 1 || c.Tracks[0].Kind != schema.KindColor || len(c.Tracks[0].Keys) != 2 {
		t.Fatalf("tracks %+v", c.Tracks)
	}
	if k := c.Tracks[0].Keys; !near(k[0].T, 0.5) || !near(k[1].T, 1.5) || k[0].Interp != curve.InterpLinear {
		t.Fatalf("pasted keys %+v", k)
	}
	if err := r.d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(r.files["assets/anim/other.aeanim"]), "track") {
		t.Fatal("one Undo must remove the pasted track")
	}
}

func TestKeyChangedKeysOnlyChangedFields(t *testing.T) {
	r := newRig(t)
	r.svc.SetMode(animedit.Preview)
	r.svc.SetTime(0.5)
	n, err := r.svc.KeyChanged(r.s.ID(r.hero))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("keyed %d fields; the scene differs from the clip on tint and position", n)
	}
	r.svc.SetMode(animedit.Normal)
	r.set("tint", schema.Color(mathx.Color{R: 0.5, G: 0, B: 0.5, A: 1}))
	r.svc.SetMode(animedit.Preview)
	r.svc.SetTime(0.5)
	tr := r.trackOf("SpriteRenderer.tint")
	before := len(r.svc.Clip().Tracks[tr].Keys)
	if _, err := r.svc.KeyChanged(r.s.ID(r.hero)); err != nil {
		t.Fatal(err)
	}
	if got := len(r.svc.Clip().Tracks[tr].Keys); got != before {
		t.Fatalf("tint already matches the clip at 0.5, it must not be keyed (%d → %d)", before, got)
	}
}

func TestSetFromScene(t *testing.T) {
	r := newRig(t)
	r.svc.SetMode(animedit.Preview)
	tr := r.trackOf("transform.position")
	if err := r.svc.SetFromScene([]animedit.KeyRef{{Track: tr, T: 1}}); err != nil {
		t.Fatal(err)
	}
	if k := r.svc.Clip().Tracks[tr].Keys[1]; k.V.L[0] != 0 || k.V.L[1] != 0 {
		t.Fatalf("key %+v must take the scene position (0, 0)", k.V)
	}
}
