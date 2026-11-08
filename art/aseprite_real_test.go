package art

import (
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// These are genuine upstream sprites, not files constructed by the importer
// tests. Each PNG was rendered independently by LibreSprite; see testdata for
// provenance, license, and the transparent-RGB limitation.
var realAsepriteFixtures = []struct {
	name, sum     string
	width, height int
	mode          Mode
	layers        []string
	frames        int
}{
	{"1empty3", "daf8e979ea8fcd4c1fbb7fef01fe6b5f7d2d67bcaf865c66716ad11807822fcf", 32, 32, RGBA, []string{"bg", "fg"}, 3},
	{"abcd", "e802c34fd630ca48a4711c6ac951b07ab0fab5fa037d74fe4f5dd2f7d44dd94b", 32, 32, Indexed, []string{"a", "b", "c", "d"}, 1},
	{"bg-index-3", "fd0a083aa13b396f6706cdc5d8567030192c829cf050543c4ac9d6d50cb772ac", 4, 4, Indexed, []string{"bg", "fg"}, 1},
	{"groups2", "55ce0dd4ba59a7a8549ccd272d878a58c69aa3edf65d75e6f68ac205df21ff19", 8, 8, Indexed, []string{"player", "body", "head", "items", "sword", "gun"}, 1},
	{"point4frames", "da1d4ca0dc1efb194e8bbefd15fd35d9eaeacc114f50012d56a223453ae35e85", 16, 16, Indexed, []string{"a", "b"}, 4},
	{"tags3", "d7729944e04e21c764e3791a64d4802a6a8529b5d1354f4b98cc9ad0488d8644", 4, 4, RGBA, []string{"a", "b", "c"}, 12},
}

func realAsepriteBytes(t *testing.T, name, sum string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "aseprite", "upstream", name+".aseprite"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != sum {
		t.Fatalf("upstream fixture %s changed: SHA-256 %s, want %s", name, got, sum)
	}
	return data
}

func realAseprite(t *testing.T, name, sum string) *Document {
	t.Helper()
	data := realAsepriteBytes(t, name, sum)
	d, notes, err := ImportAseprite(data, Limits{})
	if err != nil || len(notes) != 0 {
		t.Fatalf("import %s: diagnostics=%+v error=%v", name, notes, err)
	}
	return d
}

func compareLibreSpriteFrames(t *testing.T, name string, d *Document) {
	t.Helper()
	info := d.Info()
	for i, f := range info.Frames {
		filename := "frame.png"
		if len(info.Frames) > 1 {
			filename = fmt.Sprintf("frame%d.png", i+1)
		}
		path := filepath.Join("testdata", "aseprite", "reference", name, filename)
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		want, _, err := image.Decode(file)
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("decode reference %s: %v, close: %v", path, err, closeErr)
		}
		got, err := d.Render(f.ID, RenderOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if got.Bounds() != want.Bounds() {
			t.Fatalf("%s frame %d bounds: %v, want %v", name, i, got.Bounds(), want.Bounds())
		}
		for y := got.Bounds().Min.Y; y < got.Bounds().Max.Y; y++ {
			for x := got.Bounds().Min.X; x < got.Bounds().Max.X; x++ {
				actual := got.NRGBAAt(x, y)
				expected := color.NRGBAModel.Convert(want.At(x, y)).(color.NRGBA)
				// Composed RGB is unspecified where both pixels are fully transparent.
				if actual != expected && !(actual.A == 0 && expected.A == 0) {
					t.Fatalf("%s frame %d pixel (%d,%d): %v, want %v", name, i, x, y, actual, expected)
				}
			}
		}
	}
}

func TestRealAsepriteFilesMatchLibreSpriteAndRoundTrip(t *testing.T) {
	for _, fixture := range realAsepriteFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			d := realAseprite(t, fixture.name, fixture.sum)
			info := d.Info()
			var names []string
			for _, layer := range info.Layers {
				names = append(names, layer.Name)
			}
			if info.Canvas.Width != fixture.width || info.Canvas.Height != fixture.height || info.Canvas.Mode != fixture.mode ||
				len(info.Frames) != fixture.frames || !reflect.DeepEqual(names, fixture.layers) {
				t.Fatalf("unexpected actual source metadata: canvas=%+v frames=%d layers=%v", info.Canvas, len(info.Frames), names)
			}
			compareLibreSpriteFrames(t, fixture.name, d)
			encoded, err := d.Encode()
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := Decode(encoded, Limits{})
			if err != nil {
				t.Fatal(err)
			}
			reopened := decoded.Info()
			if info.Canvas != reopened.Canvas || !reflect.DeepEqual(info.Palette, reopened.Palette) ||
				!reflect.DeepEqual(info.Layers, reopened.Layers) || !reflect.DeepEqual(info.Frames, reopened.Frames) ||
				!reflect.DeepEqual(info.Cels, reopened.Cels) || !reflect.DeepEqual(info.Tags, reopened.Tags) ||
				!reflect.DeepEqual(info.Slices, reopened.Slices) {
				t.Fatal("source round-trip changed imported metadata or cel topology")
			}
			compareLibreSpriteFrames(t, fixture.name, decoded)
		})
	}
}

func TestRealAsepriteGroupsHiddenLayerAndEmptyFrame(t *testing.T) {
	groups := realAseprite(t, "groups2", "55ce0dd4ba59a7a8549ccd272d878a58c69aa3edf65d75e6f68ac205df21ff19").Info()
	if groups.Layers[0].Kind != Group || groups.Layers[3].Kind != Group ||
		groups.Layers[1].Parent != groups.Layers[0].ID || groups.Layers[2].Parent != groups.Layers[0].ID ||
		groups.Layers[4].Parent != groups.Layers[3].ID || groups.Layers[5].Parent != groups.Layers[3].ID ||
		groups.Layers[5].Visible {
		t.Fatalf("real group/hidden-layer structure changed: %+v", groups.Layers)
	}
	empty := realAseprite(t, "1empty3", "daf8e979ea8fcd4c1fbb7fef01fe6b5f7d2d67bcaf865c66716ad11807822fcf").Info()
	for _, cel := range empty.Cels {
		if cel.Frame == empty.Frames[1].ID {
			t.Fatal("Aseprite's empty middle frame unexpectedly contains a cel")
		}
	}
	if len(empty.Tags) != 2 || empty.Tags[0].Name != "a" || empty.Tags[1].Name != "b" ||
		empty.Tags[0].From != empty.Frames[0].ID || empty.Tags[0].To != empty.Frames[1].ID ||
		empty.Tags[1].From != empty.Frames[2].ID || empty.Tags[1].To != empty.Frames[2].ID {
		t.Fatalf("real empty-frame tag ranges changed: %+v", empty.Tags)
	}
}

func TestRealAsepriteTagsLinkedCelsAndUndo(t *testing.T) {
	fixture := realAsepriteFixtures[len(realAsepriteFixtures)-1]
	d := realAseprite(t, fixture.name, fixture.sum)
	info := d.Info()
	for i, want := range []struct {
		name        string
		direction   Direction
		first, last int
	}{{"forward", Forward, 0, 3}, {"reverse", Reverse, 4, 7}, {"pingpong", PingPong, 8, 11}} {
		if len(info.Tags) != 3 || info.Tags[i].Name != want.name || info.Tags[i].Direction != want.direction ||
			info.Tags[i].From != info.Frames[want.first].ID || info.Tags[i].To != info.Frames[want.last].ID {
			t.Fatalf("upstream animation tags do not match: %+v", info.Tags)
		}
	}
	var first, linked Cel
	for _, cel := range info.Cels {
		if cel.Layer != info.Layers[0].ID {
			continue
		}
		if cel.Frame == info.Frames[0].ID {
			first = cel
		} else if cel.Frame == info.Frames[1].ID {
			linked = cel
		}
	}
	if first.ID == "" || linked.ID == "" || first.Image != linked.Image {
		t.Fatalf("upstream linked cels were not shared: %+v %+v", first, linked)
	}
	before, err := d.Pixel(first.ID, first.X, first.Y)
	if err != nil {
		t.Fatal(err)
	}
	e := edit(t, d)
	if err := e.Unlink(linked.ID); err != nil {
		t.Fatal(err)
	}
	unlink := done(t, e)
	e = edit(t, d)
	paint := Color{R: 253, G: 2, B: 207, A: 255}
	if err := e.SetPixel(linked.ID, linked.X, linked.Y, Paint{Color: paint}, nil, false); err != nil {
		t.Fatal(err)
	}
	stroke := done(t, e)
	pixel(t, d, first.ID, first.X, first.Y, before)
	pixel(t, d, linked.ID, linked.X, linked.Y, paint)
	if err := stroke.Undo(); err != nil {
		t.Fatal(err)
	}
	if err := unlink.Undo(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(info.Cels, d.Info().Cels) {
		t.Fatal("undo did not restore the original real-file linked-cel topology")
	}
	compareLibreSpriteFrames(t, fixture.name, d)
}

func TestRealAsepriteUnsupportedTilesetIsNotSilentlyImported(t *testing.T) {
	data := realAsepriteBytes(t, "2x2tilemap2x2tile", "fbd2a4805c9adcc8be0f367332186236e053170571b69cb6c49bf4108ac5996e")
	d, _, err := ImportAseprite(data, Limits{})
	if d != nil || err == nil || !strings.Contains(err.Error(), "frame 0 chunk 0x2023: tile sets are unsupported") {
		t.Fatalf("unsupported tileset was not reported accurately: document=%v error=%v", d, err)
	}
}

func TestRealAsepriteReportsUnsupportedMetadata(t *testing.T) {
	data := realAsepriteBytes(t, "tags3x123reps", "93c0613b9cbe784368d6ab4e914abbd9deb48599541d7fb78eaded8bb8e51272")
	d, notes, err := ImportAseprite(data, Limits{})
	if err != nil || d == nil || len(d.Info().Tags) != 3 {
		t.Fatalf("repeat-count fixture did not import its tags: %v, notes=%+v", err, notes)
	}
	counts := map[string]int{}
	for _, note := range notes {
		if note.Message == "" || note.Frame != 0 {
			t.Fatalf("incomplete metadata diagnostic: %+v", note)
		}
		counts[note.Code]++
	}
	if counts["tag-repeat"] != 3 || counts["metadata-skipped"] != 3 {
		t.Fatalf("discarded finite repeats or user data were not reported: %+v", notes)
	}
	data = realAsepriteBytes(t, "slices", "4215a4967b83533c3bcaad8a23e76f12a6b2049081eda730d0e6d38f78f76582")
	d, notes, err = ImportAseprite(data, Limits{})
	if d != nil || err == nil || !strings.Contains(err.Error(), "indexed background layer is not supported") || len(notes) == 0 {
		t.Fatalf("indexed background was silently converted: document=%v error=%v notes=%+v", d, err, notes)
	}
	data = realAsepriteBytes(t, "file-tests-props", "657e2549cd74a94137aec6502957d18662f4e4d50f565009d60f9a4189b22087")
	d, notes, err = ImportAseprite(data, Limits{})
	if d != nil || err == nil || !strings.Contains(err.Error(), "tile sets are unsupported") || len(notes) < 2 {
		t.Fatalf("mixed user metadata and tilemap were silently accepted: document=%v error=%v notes=%+v", d, err, notes)
	}
}
