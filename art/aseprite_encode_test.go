package art

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"image"
	"io"
	"reflect"
	"strings"
	"testing"
)

// Check the wire header and decompressed cel independently of ImportAseprite;
// a self-consistent pair of incorrect codecs must not pass by itself.
func TestAsepriteEncoderWritesRealHeaderAndCelBytes(t *testing.T) {
	for _, tc := range []struct {
		mode  Mode
		depth uint16
		paint Paint
		pixel []byte
	}{
		{RGBA, 32, Paint{Color: Color{3, 14, 251, 127}}, []byte{3, 14, 251, 127}},
		{Grayscale, 16, Paint{Color: Color{87, 87, 87, 199}}, []byte{87, 199}},
		{Indexed, 8, Paint{UseIndex: true, Index: 3}, []byte{3}},
	} {
		t.Run(string(tc.mode), func(t *testing.T) {
			d, err := New(2, 2, tc.mode, Limits{})
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			cel := d.Info().Cels[0].ID
			e := edit(t, d)
			if err := e.SetPixel(cel, 0, 0, tc.paint, nil, false); err != nil {
				t.Fatal(err)
			}
			done(t, e)
			before := d.Info()
			s, err := d.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			data, notes, err := s.EncodeAseprite()
			if err != nil || len(notes) != 1 || notes[0].Code != "aego-identities" {
				t.Fatalf("export: %+v %v", notes, err)
			}
			if !reflect.DeepEqual(before, d.Info()) {
				t.Fatal("encoding changed the document")
			}
			if len(data) < 144 || binary.LittleEndian.Uint32(data) != uint32(len(data)) || binary.LittleEndian.Uint16(data[4:]) != 0xa5e0 || binary.LittleEndian.Uint16(data[12:]) != tc.depth || bytes.HasPrefix(data, []byte("PK")) {
				t.Fatal("not an Aseprite header")
			}
			if binary.LittleEndian.Uint16(data[132:]) != 0xf1fa {
				t.Fatal("not an Aseprite frame")
			}
			count := int(binary.LittleEndian.Uint32(data[140:]))
			at, found := 144, false
			for i := 0; i < count; i++ {
				if at+6 > len(data) {
					t.Fatal("truncated chunk")
				}
				n := int(binary.LittleEndian.Uint32(data[at:]))
				if n < 6 || at+n > len(data) {
					t.Fatal("invalid chunk size")
				}
				if binary.LittleEndian.Uint16(data[at+4:]) == 0x2005 {
					b := data[at+6 : at+n]
					if len(b) < 20 || binary.LittleEndian.Uint16(b[7:]) != 2 || binary.LittleEndian.Uint16(b[16:]) != 2 || binary.LittleEndian.Uint16(b[18:]) != 2 {
						t.Fatal("not a compressed 2x2 cel")
					}
					z, err := zlib.NewReader(bytes.NewReader(b[20:]))
					if err != nil {
						t.Fatal(err)
					}
					raw, err := io.ReadAll(z)
					if err != nil {
						t.Fatal(err)
					}
					if err := z.Close(); err != nil {
						t.Fatal(err)
					}
					if len(raw) != int(tc.depth/8)*4 || !bytes.Equal(raw[:len(tc.pixel)], tc.pixel) {
						t.Fatalf("wrong raw pixels: %v", raw)
					}
					found = true
				}
				at += n
			}
			if !found || at != len(data) {
				t.Fatal("cel missing or unexpected trailing bytes")
			}
			q, importNotes, err := ImportAseprite(data, Limits{})
			if err != nil || len(importNotes) != 0 {
				t.Fatalf("reimport: %+v %v", importNotes, err)
			}
			defer q.Close()
			got, err := q.Pixel(q.Info().Cels[0].ID, 0, 0)
			if err != nil || got != decode(d.current, d.current.Images[d.current.Cels[0].Image].at(0, 0)) {
				t.Fatalf("reimport pixel: %+v %v", got, err)
			}
		})
	}
}

func TestAsepriteEncoderLayersFramesLinksTagsAndStaticPivot(t *testing.T) {
	d, layer, first, cel := doc(t, RGBA)
	defer d.Close()
	e := edit(t, d)
	group, err := e.AddLayer("Group", Group, "")
	if err != nil {
		t.Fatal(err)
	}
	l := e.Info().Layers[0]
	l.Parent, l.Blend, l.Opacity = group, BlendMultiply, 211
	if err := e.SetLayer(l); err != nil {
		t.Fatal(err)
	}
	if _, err := e.AddLayer("Foreground", Raster, ""); err != nil {
		t.Fatal(err)
	}
	if err := e.SetPixel(cel, 1, 2, Paint{Color: red}, nil, false); err != nil {
		t.Fatal(err)
	}
	frames := []ID{first}
	for i := 1; i < 8; i++ {
		frame, err := e.AddFrame(frames[i-1], 100+i*7)
		if err != nil {
			t.Fatal(err)
		}
		frames = append(frames, frame)
		if i == 1 {
			linked, err := e.CopyCel(cel, layer, frame, true)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.SetCelPosition(linked, -1, 3); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := e.SetTag(Tag{Name: "Walk", From: frames[0], To: frames[3], Direction: Forward}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SetTag(Tag{Name: "Return", From: frames[4], To: frames[7], Direction: PingPong}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SetSlice(Slice{Name: "pivot", Rect: image.Rect(2, 3, 10, 12), Pivot: image.Pt(6, 8)}); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	s, err := d.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	data, notes, err := s.EncodeAseprite()
	if err != nil || len(notes) != 1 {
		t.Fatalf("export: %+v %v", notes, err)
	}
	q, importNotes, err := ImportAseprite(data, Limits{})
	if err != nil || len(importNotes) != 0 {
		t.Fatalf("reimport: %+v %v", importNotes, err)
	}
	defer q.Close()
	info := q.Info()
	if len(info.Layers) != 3 || info.Layers[0].Name != "Group" || info.Layers[1].Parent != info.Layers[0].ID || info.Layers[1].Blend != BlendMultiply || info.Layers[1].Opacity != 211 || info.Layers[2].Name != "Foreground" {
		t.Fatalf("layer hierarchy: %+v", info.Layers)
	}
	if len(info.Frames) != 8 || len(info.Tags) != 2 || info.Tags[0].From != info.Frames[0].ID || info.Tags[1].To != info.Frames[7].ID || info.Tags[1].Direction != PingPong {
		t.Fatalf("frames/tags: %+v %+v", info.Frames, info.Tags)
	}
	for i := 1; i < 8; i++ {
		if info.Frames[i].DurationMS != 100+i*7 {
			t.Fatalf("frame duration %d: %d", i, info.Frames[i].DurationMS)
		}
	}
	if len(info.Cels) != 2 || info.Cels[0].Image != info.Cels[1].Image || info.Cels[1].X != -1 || info.Cels[1].Y != 3 {
		t.Fatalf("linked cels: %+v", info.Cels)
	}
	if len(info.Slices) != 1 || info.Slices[0].Frame != "" || info.Slices[0].Rect != image.Rect(2, 3, 10, 12) || info.Slices[0].Pivot != image.Pt(6, 8) {
		t.Fatalf("static slice: %+v", info.Slices)
	}
	for _, frame := range info.Frames {
		if pivot, ok := info.FramePivot(frame.ID); !ok || pivot != image.Pt(6, 8) {
			t.Fatalf("pivot not global on frame %s", frame.ID)
		}
	}
}

func TestAsepriteEncoderRejectsUnrepresentedDataWithoutMutatingSource(t *testing.T) {
	for _, tc := range []struct {
		name, diagnostic string
		modify           func(*testing.T, *Edit, ID, ID, ID)
	}{
		{"extra surface", "surface-channel", func(t *testing.T, e *Edit, l, f, _ ID) {
			if _, err := e.EnsureCel(l, f, Normal); err != nil {
				t.Fatal(err)
			}
		}},
		{"alpha lock", "alpha-lock", func(t *testing.T, e *Edit, _, _, _ ID) {
			l := e.Info().Layers[0]
			l.AlphaLock = true
			if err := e.SetLayer(l); err != nil {
				t.Fatal(err)
			}
		}},
		{"properties", "properties", func(_ *testing.T, e *Edit, _, _, _ ID) {
			e.work.Properties["aego.example"] = "keep"
			e.structural = true
		}},
		{"cross-layer link", "cross-layer-link", func(t *testing.T, e *Edit, _, f, c ID) {
			l, err := e.AddLayer("Other", Raster, "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.CopyCel(c, l, f, true); err != nil {
				t.Fatal(err)
			}
		}},
		{"long duration", "frame-duration", func(t *testing.T, e *Edit, _, f, _ ID) {
			if err := e.SetDuration(f, 70000); err != nil {
				t.Fatal(err)
			}
		}},
		{"frame slice", "slice-timing", func(t *testing.T, e *Edit, _, f, _ ID) {
			if _, err := e.AddFrame(f, 100); err != nil {
				t.Fatal(err)
			}
			if _, err := e.SetSlice(Slice{Name: "pivot", Frame: f, Rect: image.Rect(0, 0, 16, 16)}); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, l, f, c := doc(t, RGBA)
			defer d.Close()
			e := edit(t, d)
			tc.modify(t, e, l, f, c)
			done(t, e)
			before := d.Info()
			s, err := d.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			data, notes, err := s.EncodeAseprite()
			if err == nil || data != nil || len(notes) == 0 || notes[0].Code != tc.diagnostic {
				t.Fatalf("did not reject %s: %v %+v %v", tc.name, data, notes, err)
			}
			if !reflect.DeepEqual(before, d.Info()) {
				t.Fatal("failed export changed the source")
			}
		})
	}
}

func TestAsepriteEncoderUsesOneSnapshotAndHonorsOutputLimit(t *testing.T) {
	d, _, _, cel := doc(t, RGBA)
	defer d.Close()
	s, err := d.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	e := edit(t, d)
	if err := e.SetPixel(cel, 0, 0, Paint{Color: red}, nil, false); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	data, _, err := s.EncodeAseprite()
	if err != nil {
		t.Fatal(err)
	}
	q, _, err := ImportAseprite(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	pixel(t, q, q.Info().Cels[0].ID, 0, 0, Color{})
	s.view.limits.MaxFileBytes = 128
	data, _, err = s.EncodeAseprite()
	if data != nil || !errors.Is(err, ErrLimit) {
		t.Fatalf("output limit: %v %v", data, err)
	}
	if err := d.Publish(s, func() error { t.Fatal("stale snapshot reached publication"); return nil }); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale snapshot: %v", err)
	}
}

func TestAsepriteImporterReportsPersistentSliceKeys(t *testing.T) {
	w := aseFixture{}
	w.u32(2)
	w.u32(2)
	w.u32(0)
	w.str("pivot")
	for frame := uint32(0); frame < 2; frame++ {
		w.u32(frame)
		w.u32(3)
		w.u32(4)
		w.u32(4)
		w.u32(4)
		w.u32(2)
		w.u32(1)
	}
	data := aseFile(32, 0, aseFrame(100, aseLayerChunk("L", 0, 0, 255), aseChunk(0x2022, w.Bytes())), aseFrame(100))
	d, notes, err := ImportAseprite(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if len(notes) != 1 || notes[0].Code != "slice-keyframe-timing" || !strings.Contains(notes[0].Message, "persist") {
		t.Fatalf("missing timing warning: %+v", notes)
	}
	if slices := d.Info().Slices; len(slices) != 2 || slices[0].Pivot != image.Pt(5, 5) {
		t.Fatalf("relative pivot: %+v", slices)
	}
}
