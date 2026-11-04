package propref

import (
	"strings"
	"testing"

	"aego/core/mathx"
	"aego/scene"
	"aego/schema"
)

type laneProbe struct {
	Tint mathx.Color `aego:"tint"`
}

type qualifiedLaneProbe struct {
	Alpha float32 `aego:"a"`
}

func laneScene(t *testing.T, qualified bool) (*scene.Scene, scene.NodeHandle, *Resolver) {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := scene.RegisterComponent[laneProbe](types, schema.Name("test.Probe")); err != nil {
		t.Fatal(err)
	}
	if qualified {
		if err := scene.RegisterComponent[qualifiedLaneProbe](types, schema.Name("test.Probe.tint")); err != nil {
			t.Fatal(err)
		}
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	h := s.Create(s.Root(), "probe")
	scene.Add[laneProbe](s, h).Tint = mathx.Color{A: 0.25}
	if qualified {
		scene.Add[qualifiedLaneProbe](s, h).Alpha = 0.75
	}
	s.Flush()
	return s, h, New(s, nil)
}

func TestResolveVectorLaneDoesNotAllocateSpeculativeDiagnostics(t *testing.T) {
	_, h, r := laneScene(t, false)
	for _, path := range []string{"Probe.tint.a", "test.Probe.tint.a", "transform.position.x"} {
		t.Run(path, func(t *testing.T) {
			for _, discrete := range []bool{false, true} {
				ref, err := r.resolve(h, path, discrete)
				if err != nil || ref.Lane() < 0 {
					t.Fatalf("resolve lane: ref=%+v err=%v", ref, err)
				}
				if n := testing.AllocsPerRun(100, func() {
					resolved, err := r.resolve(h, path, discrete)
					if err != nil || !resolved.Same(&ref) {
						t.Fatalf("rebind: ref=%+v err=%v", resolved, err)
					}
				}); n != 0 {
					t.Fatalf("successful lane binding allocated %g diagnostics per resolution", n)
				}
			}
		})
	}
}

func TestResolveQualifiedComponentPrecedesLane(t *testing.T) {
	_, h, r := laneScene(t, true)
	ref, err := r.ResolveAny(h, "test.Probe.tint.a")
	if err != nil {
		t.Fatal(err)
	}
	value, ok := r.Read(&ref, h)
	if !ok || ref.Lane() != -1 || value[0] != 0.75 {
		t.Fatalf("qualified scalar field must precede Probe.tint alpha lane: ref=%+v value=%v ok=%v", ref, value, ok)
	}
}

func TestResolveInvalidLaneStillExplainsFailure(t *testing.T) {
	_, h, r := laneScene(t, false)
	for _, path := range []string{"Missing.tint.a", "Probe.missing.a", "transform.position.w"} {
		_, err := r.ResolveAny(h, path)
		if err == nil || !strings.Contains(err.Error(), path) {
			t.Fatalf("invalid property %q lost its diagnostic: %v", path, err)
		}
	}
}
