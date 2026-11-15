package artsource

import (
	"context"
	"image"
	"math"
	"testing"

	"aego/anim"
	"aego/art"
	"aego/asset"
)

func TestLinkedFramesAndTaggedClipUseRuntimeAnimation(t *testing.T) {
	ctx, session := testSource(t)
	defer session.Document.Close()
	exec := func(op art.Operation) art.ID {
		t.Helper()
		id, err := session.Execute(context.Background(), op)
		if err != nil {
			t.Fatalf("%s: %v", op.Kind, err)
		}
		return id
	}
	paint := func(c art.Color) {
		t.Helper()
		exec(art.Operation{Kind: "stroke", Points: []image.Point{{X: 2, Y: 2}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: c}}})
	}
	pixel := func(frame art.ID) art.Color {
		t.Helper()
		img, err := session.Document.Render(frame, art.RenderOptions{Channel: art.Albedo})
		if err != nil {
			t.Fatal(err)
		}
		c := img.NRGBAAt(2, 2)
		return art.Color{R: c.R, G: c.G, B: c.B, A: c.A}
	}
	first := session.Frame
	exec(art.Operation{Kind: "duration", DurationMS: 50})
	red := art.Color{R: 255, A: 255}
	blue := art.Color{B: 255, A: 255}
	green := art.Color{G: 255, A: 255}
	paint(red)
	second := exec(art.Operation{Kind: "duplicate-frame", Linked: true})
	exec(art.Operation{Kind: "duration", DurationMS: 70})
	paint(blue)
	if pixel(first) != blue || pixel(second) != blue {
		t.Fatal("linked frame edits did not affect both cels")
	}
	exec(art.Operation{Kind: "unlink"})
	paint(red)
	if pixel(first) != blue || pixel(second) != red {
		t.Fatal("unlinked cel remained shared")
	}
	third := exec(art.Operation{Kind: "duplicate-frame"})
	exec(art.Operation{Kind: "duration", DurationMS: 210})
	paint(green)
	if pixel(second) != red || pixel(third) != green {
		t.Fatal("independent duplicated frame shared pixels")
	}
	tag := exec(art.Operation{Kind: "tag", Tag: &art.Tag{Name: "walk", From: first, To: third, Direction: art.PingPong}})
	var err error
	ctx.Source, err = session.Document.Encode()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := art.Decode(ctx.Source, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if len(reopened.Info().Frames) != 3 || len(reopened.Info().Tags) != 1 {
		t.Fatal("source round trip lost animation structure")
	}
	result, err := New().Import(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var data []byte
	for _, sub := range result.Subs {
		if sub.ID == StableID(ctx.ID, ClipName(tag)) {
			if sub.Kind != asset.ArtAnim || len(sub.Artifacts) != 1 || sub.Artifacts[0].Kind != asset.ArtAnim {
				t.Fatal("tag did not produce an engine animation artifact")
			}
			data = sub.Artifacts[0].Data
		}
	}
	clip, err := anim.Unmarshal(data)
	if err != nil {
		t.Fatalf("existing runtime cannot decode the tag clip: %v", err)
	}
	if len(clip.Tracks) != 1 || clip.Tracks[0].Property != anim.SpriteProp || clip.Loop != anim.LoopRepeat || math.Abs(float64(clip.Length)-0.4) > 1e-6 {
		t.Fatalf("unexpected runtime clip: %+v", clip)
	}
	want := []struct {
		at    float64
		frame art.ID
	}{{0.025, first}, {0.085, second}, {0.225, third}, {0.365, second}, {0.425, first}}
	var sampler anim.Sampler
	sampler.Reset(clip)
	for _, tc := range want {
		local, _ := clip.Local(clip.Loop, tc.at)
		got := sampler.Track(clip, 0, local).A
		expected := StableID(ctx.ID, FrameName(tc.frame, art.Albedo))
		if got != expected {
			t.Fatalf("runtime at %.3fs: got %s, want frame %s", tc.at, got, expected)
		}
	}
}
