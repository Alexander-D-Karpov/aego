package gpupreview

import (
	"bytes"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"aego/art"
	"aego/asset"
)

// The old per-frame scan defines which channels and ordering are published.
// Exercise duplicates, missing channels and varied cel order against that rule.
func TestSurfaceChannelsMatchPerFrameScan(t *testing.T) {
	rng := rand.New(rand.NewSource(1205))
	choices := []art.Channel{art.Albedo, art.Normal, art.Height, art.Emission}
	for trial := range 100 {
		info := art.Info{Canvas: art.Canvas{Width: 4, Height: 4}}
		for n := range 1 + rng.Intn(20) {
			info.Frames = append(info.Frames, art.Frame{ID: art.ID(string(rune('a' + n)))})
		}
		for range rng.Intn(300) {
			info.Cels = append(info.Cels, art.Cel{
				Frame:   info.Frames[rng.Intn(len(info.Frames))].ID,
				Channel: choices[rng.Intn(len(choices))],
			})
		}
		want := make(map[art.ID][]art.Channel, len(info.Frames))
		for _, f := range info.Frames {
			want[f.ID] = []art.Channel{art.Albedo}
			for _, ch := range choices[1:] {
				for _, c := range info.Cels {
					if c.Frame == f.ID && c.Channel == ch {
						want[f.ID] = append(want[f.ID], ch)
						break
					}
				}
			}
		}
		got, err := surfaceChannels(info)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("trial %d: channel indexing differs: got=%v want=%v err=%v", trial, got, want, err)
		}
	}
}

func TestSurfaceAndSnapshotBudgets(t *testing.T) {
	// Four 2048x2048 RGBA surfaces are exactly 64 MiB. Duplicate cels for
	// the same frame/channel are still one composited surface.
	info := art.Info{
		Canvas: art.Canvas{Width: 2048, Height: 2048},
		Frames: []art.Frame{{ID: "one"}, {ID: "two"}, {ID: "three"}},
		Cels:   []art.Cel{{Frame: "one", Channel: art.Normal}, {Frame: "one", Channel: art.Normal}},
	}
	channels, err := surfaceChannels(info)
	if err != nil || len(channels["one"]) != 2 {
		t.Fatal("exact pixel budget or duplicate channel handling changed", channels, err)
	}
	info.Cels = append(info.Cels, art.Cel{Frame: "two", Channel: art.Emission})
	if _, err = surfaceChannels(info); err == nil || !strings.Contains(err.Error(), "64 MiB budget") {
		t.Fatal("accepted a prepared surface above the existing 64 MiB budget", err)
	}
	if _, err = prepare(nil); err == nil {
		t.Fatal("accepted an empty snapshot")
	}
	if _, err = prepare(make([]byte, MaxSnapshotBytes+1)); err == nil {
		t.Fatal("accepted a snapshot above the existing 64 MiB limit")
	}
}

func TestPreparedArtifactsOutliveTheirSourceBytes(t *testing.T) {
	d, data := fixture(t)
	prepared, err := prepare(data)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.info.ID != d.Info().ID || prepared.info.Revision != d.Revision() {
		t.Fatal("prepared metadata did not capture the original document")
	}
	var texturePath string
	for _, entry := range prepared.entries {
		if entry.Kind == asset.ArtTexture {
			texturePath = entry.Artifacts[0].Path
			break
		}
	}
	if texturePath == "" {
		t.Fatal("prepared frame has no texture artifact")
	}
	before, err := prepared.files.ReadFile(texturePath)
	if err != nil {
		t.Fatal(err)
	}
	// The producer may release or overwrite its input as soon as Publish returns.
	clear(data)
	d.Close()
	after, err := prepared.files.ReadFile(texturePath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("prepared artifacts retained mutable input/source ownership", err)
	}
	tex, err := asset.DecodeTexture(after)
	if err != nil || tex.Width != 16 || tex.Height != 16 || tex.Pixels[(6*16+5)*4+3] != 128 {
		t.Fatal("detached prepared pixels are unusable", err)
	}
}
