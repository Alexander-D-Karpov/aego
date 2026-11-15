package art

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"reflect"
	"sync"
	"testing"
)

var red = Color{255, 0, 0, 255}
var blue = Color{0, 0, 255, 255}

func doc(t testing.TB, mode Mode) (*Document, ID, ID, ID) {
	t.Helper()
	d, err := New(16, 16, mode, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	i := d.Info()
	return d, i.Layers[0].ID, i.Frames[0].ID, i.Cels[0].ID
}
func edit(t testing.TB, d *Document) *Edit {
	t.Helper()
	e, err := d.Begin(d.Revision(), "test")
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func done(t testing.TB, e *Edit) *Change {
	t.Helper()
	c, err := e.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if c != nil {
		if err = c.Apply(); err != nil {
			t.Fatal(err)
		}
	}
	return c
}
func pixel(t testing.TB, d *Document, c ID, x, y int, want Color) {
	t.Helper()
	v, err := d.Pixel(c, x, y)
	if err != nil || v != want {
		t.Fatalf("pixel %d,%d = %+v, %v; want %+v", x, y, v, err, want)
	}
}
func brush(c Color) Brush { return Brush{Size: 1, Paint: Paint{Color: c}} }
func hash(t testing.TB, d *Document, f ID) [32]byte {
	t.Helper()
	im, err := d.Render(f, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(im.Pix)
}

func TestStrokeInterpolationAndSingleUndo(t *testing.T) {
	d, _, _, c := doc(t, RGBA)
	e := edit(t, d)
	g, err := e.Stroke(c, brush(red), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []image.Point{{0, 0}, {15, 0}, {15, 15}} {
		if err = g.Point(p); err != nil {
			t.Fatal(err)
		}
	}
	pixel(t, d, c, 5, 0, Color{}) // Unconfirmed preview cannot leak into canonical pixels.
	ch, err := e.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	j := NewJournal(Limits{})
	if err = j.Exec(ch); err != nil {
		t.Fatal(err)
	}
	for x := 0; x < 16; x++ {
		pixel(t, d, c, x, 0, red)
	}
	for y := 0; y < 16; y++ {
		pixel(t, d, c, 15, y, red)
	}
	if u, r := j.Counts(); u != 1 || r != 0 {
		t.Fatal(u, r)
	}
	rev := d.Revision()
	if err = j.Undo(); err != nil {
		t.Fatal(err)
	}
	pixel(t, d, c, 5, 0, Color{})
	if d.Revision() <= rev {
		t.Fatal("revision decreased on undo")
	}
	if err = j.Redo(); err != nil {
		t.Fatal(err)
	}
	pixel(t, d, c, 5, 0, red)
}
func TestPixelPerfectRemovesCornerOnly(t *testing.T) {
	d, _, _, c := doc(t, RGBA)
	e := edit(t, d)
	b := brush(red)
	b.PixelPerfect = true
	g, err := e.Stroke(c, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []image.Point{{2, 2}, {3, 2}, {3, 3}} {
		if err = g.Point(p); err != nil {
			t.Fatal(err)
		}
	}
	done(t, e)
	pixel(t, d, c, 2, 2, red)
	pixel(t, d, c, 3, 2, Color{})
	pixel(t, d, c, 3, 3, red)
}
func TestCancelAndNoOpDoNotChangeRevision(t *testing.T) {
	d, _, _, c := doc(t, RGBA)
	rev := d.Revision()
	e := edit(t, d)
	if err := e.SetPixel(c, 1, 1, Paint{Color: red}, nil, false); err != nil {
		t.Fatal(err)
	}
	e.Cancel()
	if d.Revision() != rev {
		t.Fatal("cancel changed revision")
	}
	if _, err := e.Prepare(); !errors.Is(err, ErrDone) {
		t.Fatal(err)
	}
	e = edit(t, d)
	ch, err := e.Prepare()
	if err != nil || ch != nil {
		t.Fatal(ch, err)
	}
}
func TestLinkedCelsDuplicateUnlinkAndUndo(t *testing.T) {
	d, l, f, c := doc(t, RGBA)
	e := edit(t, d)
	f2, err := e.AddFrame(f, 100)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := e.CopyCel(c, l, f2, true)
	if err != nil {
		t.Fatal(err)
	}
	f3, err := e.AddFrame(f2, 100)
	if err != nil {
		t.Fatal(err)
	}
	independent, err := e.CopyCel(c, l, f3, false)
	if err != nil {
		t.Fatal(err)
	}
	done(t, e)
	e = edit(t, d)
	if err = e.SetPixel(c, 2, 2, Paint{Color: red}, nil, false); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	pixel(t, d, linked, 2, 2, red)
	pixel(t, d, independent, 2, 2, Color{})
	e = edit(t, d)
	if err = e.Unlink(linked); err != nil {
		t.Fatal(err)
	}
	unlink := done(t, e)
	e = edit(t, d)
	if err = e.SetPixel(c, 2, 2, Paint{Color: blue}, nil, false); err != nil {
		t.Fatal(err)
	}
	paint := done(t, e)
	pixel(t, d, linked, 2, 2, red)
	if err = paint.Undo(); err != nil {
		t.Fatal(err)
	}
	if err = unlink.Undo(); err != nil {
		t.Fatal(err)
	}
	e = edit(t, d)
	if err = e.SetPixel(c, 2, 2, Paint{Color: blue}, nil, false); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	pixel(t, d, linked, 2, 2, blue)
}
func TestIndependentImagesUseCopyOnWrite(t *testing.T) {
	d, l, f, c := doc(t, RGBA)
	e := edit(t, d)
	_ = e.SetPixel(c, 1, 1, Paint{Color: red}, nil, false)
	done(t, e)
	e = edit(t, d)
	f2, _ := e.AddFrame(f, 100)
	copyID, err := e.CopyCel(c, l, f2, false)
	if err != nil {
		t.Fatal(err)
	}
	done(t, e)
	s := d.current
	a, _ := s.cel(c)
	b, _ := s.cel(copyID)
	p, q := s.Images[s.Cels[a].Image], s.Images[s.Cels[b].Image]
	if s.Cels[a].Image == s.Cels[b].Image || p != q {
		t.Fatal("copy did not share immutable COW pixels")
	}
	e = edit(t, d)
	_ = e.SetPixel(copyID, 1, 1, Paint{Color: blue}, nil, false)
	done(t, e)
	pixel(t, d, c, 1, 1, red)
	pixel(t, d, copyID, 1, 1, blue)
}
func TestIndexedReorderPreservesPixelsAndRemapChangesThem(t *testing.T) {
	d, _, f, c := doc(t, Indexed)
	e := edit(t, d)
	if err := e.SetPixel(c, 3, 4, Paint{Index: 6, UseIndex: true}, nil, false); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	before := hash(t, d, f)
	order := make([]int, len(d.Info().Palette))
	for i := range order {
		order[i] = i
	}
	order[6], order[7] = 7, 6
	e = edit(t, d)
	if err := e.ReorderPalette(order); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	if hash(t, d, f) != before {
		t.Fatal("reorder changed visible colors")
	}
	e = edit(t, d)
	mapping := make([]int, len(order))
	for i := range mapping {
		mapping[i] = i
	}
	mapping[7] = 8
	if err := e.RemapPalette(mapping); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	if hash(t, d, f) == before {
		t.Fatal("remap did not change color")
	}
}
func TestGrayscaleAndAlphaLock(t *testing.T) {
	d, l, _, c := doc(t, Grayscale)
	e := edit(t, d)
	_ = e.SetPixel(c, 2, 2, Paint{Color: Color{255, 0, 0, 100}}, nil, false)
	done(t, e)
	pixel(t, d, c, 2, 2, Color{77, 77, 77, 100})
	e = edit(t, d)
	lv := d.Info().Layers[0]
	lv.AlphaLock = true
	if err := e.SetLayer(lv); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	e = edit(t, d)
	_ = e.SetPixel(c, 2, 2, Paint{Color: blue}, nil, false)
	_ = e.SetPixel(c, 3, 3, Paint{Color: red}, nil, false)
	done(t, e)
	pixel(t, d, c, 2, 2, Color{29, 29, 29, 100})
	pixel(t, d, c, 3, 3, Color{})
	e = edit(t, d)
	lv = d.Info().Layers[0]
	lv.Locked = true
	_ = e.SetLayer(lv)
	done(t, e)
	e = edit(t, d)
	if err := e.SetPixel(c, 2, 2, Paint{Color: red}, nil, false); !errors.Is(err, ErrLocked) {
		t.Fatal(l, err)
	}
}
func TestGroupsOpacityReferenceAndSelection(t *testing.T) {
	d, l, f, c := doc(t, RGBA)
	e := edit(t, d)
	g, err := e.AddLayer("group", Group, "")
	if err != nil {
		t.Fatal(err)
	}
	lv := e.Info().Layers[0]
	lv.Parent = g
	_ = e.SetLayer(lv)
	_ = e.SetPixel(c, 1, 1, Paint{Color: red}, nil, false)
	top, _ := e.AddLayer("top", Raster, g)
	tc, _ := e.EnsureCel(top, f, Albedo)
	_ = e.SetPixel(tc, 1, 1, Paint{Color: blue}, nil, false)
	gi, _ := e.work.layer(g)
	gv := e.work.Layers[gi]
	gv.Opacity = 128
	_ = e.SetLayer(gv)
	ref, _ := e.AddLayer("reference", Reference, "")
	rc, _ := e.EnsureCel(ref, f, Albedo)
	_ = e.SetPixel(rc, 1, 1, Paint{Color: red}, nil, false)
	done(t, e)
	im, err := d.Render(f, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v := im.NRGBAAt(1, 1); v != (color.NRGBA{B: 255, A: 128}) {
		t.Fatal("group opacity applied per child", v)
	}
	im, err = d.Render(f, RenderOptions{References: true})
	if err != nil {
		t.Fatal(err)
	}
	if im.NRGBAAt(1, 1).R != 255 {
		t.Fatal("reference missing")
	}
	mask, _ := RectSelection(16, 16, image.Rect(5, 5, 7, 7), false)
	e = edit(t, d)
	if err = e.Shape(c, "rectangle", image.Pt(0, 0), image.Pt(15, 15), brush(red), mask, true); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	pixel(t, d, c, 5, 5, red)
	pixel(t, d, c, 4, 5, Color{})
	_ = l
}
func TestFillWandAndShapes(t *testing.T) {
	d, _, _, c := doc(t, RGBA)
	e := edit(t, d)
	if err := e.Shape(c, "rectangle", image.Pt(3, 3), image.Pt(10, 10), brush(red), nil, false); err != nil {
		t.Fatal(err)
	}
	if err := e.Fill(context.Background(), c, 5, 5, 0, Paint{Color: blue}, nil, true); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	pixel(t, d, c, 4, 4, blue)
	pixel(t, d, c, 2, 2, Color{})
	pixel(t, d, c, 3, 3, red)
	sel, err := d.Wand(c, 5, 5, 0, true)
	if err != nil || sel.Count() != 36 {
		t.Fatal(sel.Count(), err)
	}
	e = edit(t, d)
	if err = e.Shape(c, "ellipse", image.Pt(0, 0), image.Pt(2, 2), brush(red), nil, true); err != nil {
		t.Fatal(err)
	}
	done(t, e)
}
func TestSelectionTransformClipboard(t *testing.T) {
	d, _, _, c := doc(t, RGBA)
	e := edit(t, d)
	_ = e.SetPixel(c, 1, 1, Paint{Color: red}, nil, false)
	_ = e.SetPixel(c, 2, 1, Paint{Color: blue}, nil, false)
	done(t, e)
	sel, _ := RectSelection(16, 16, image.Rect(1, 1, 3, 2), false)
	e = edit(t, d)
	if err := e.Transform(c, sel, "flip-x", 0, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	pixel(t, d, c, 1, 1, blue)
	pixel(t, d, c, 2, 1, red)
	clip, err := d.Copy(c, sel)
	if err != nil {
		t.Fatal(err)
	}
	rot, err := clip.Transform("rotate-cw", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rot.Width != 1 || rot.Height != 2 {
		t.Fatal(rot)
	}
	e = edit(t, d)
	if err = e.Paste(c, rot, image.Pt(5, 5), nil); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	pixel(t, d, c, 5, 5, blue)
	pixel(t, d, c, 5, 6, red)
}
func TestTagTimingPingPongAndDelete(t *testing.T) {
	d, _, f, _ := doc(t, RGBA)
	e := edit(t, d)
	f2, _ := e.AddFrame(f, 200)
	f3, _ := e.AddFrame(f2, 300)
	tag, err := e.SetTag(Tag{Name: "walk", From: f, To: f3, Direction: PingPong})
	if err != nil {
		t.Fatal(err)
	}
	done(t, e)
	for _, v := range []struct {
		ms int64
		id ID
	}{{0, f}, {99, f}, {100, f2}, {300, f3}, {600, f2}, {799, f2}, {800, f}, {-1, f2}} {
		id, err := d.FrameAt(tag, v.ms)
		if err != nil || id != v.id {
			t.Fatal(v, id, err)
		}
	}
	e = edit(t, d)
	if err = e.DeleteFrame(f); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	if d.Info().Tags[0].From != f2 {
		t.Fatal("tag not retargeted")
	}
}
func TestRevisionConflictCloseAndSavedCheckpoint(t *testing.T) {
	d, _, _, c := doc(t, RGBA)
	saved, err := d.Checkpoint(d.Revision())
	if err != nil {
		t.Fatal(err)
	}
	a, b := edit(t, d), edit(t, d)
	_ = a.SetPixel(c, 0, 0, Paint{Color: red}, nil, false)
	_ = b.SetPixel(c, 0, 0, Paint{Color: blue}, nil, false)
	ca, err := a.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	cb, err := b.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if err = ca.Apply(); err != nil {
		t.Fatal(err)
	}
	if err = cb.Apply(); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if !d.DirtySince(saved) {
		t.Fatal("missing dirty state")
	}
	if err = ca.Undo(); err != nil {
		t.Fatal(err)
	}
	if d.DirtySince(saved) {
		t.Fatal("undo did not return to saved state")
	}
	if err = cb.Apply(); !errors.Is(err, ErrConflict) {
		t.Fatal("stale proposal applied after undo", err)
	}
	d.Close()
	if err = ca.Apply(); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}
func TestIDsNotReusedAfterBranchingUndo(t *testing.T) {
	d, _, f, _ := doc(t, RGBA)
	e := edit(t, d)
	first, _ := e.AddFrame(f, 100)
	ch := done(t, e)
	if err := ch.Undo(); err != nil {
		t.Fatal(err)
	}
	e = edit(t, d)
	second, _ := e.AddFrame(f, 100)
	done(t, e)
	if first == second {
		t.Fatal("frame ID was reused after Undo")
	}
}
func TestFormatRoundtripAndLinks(t *testing.T) {
	d, l, f, c := doc(t, Indexed)
	e := edit(t, d)
	_ = e.SetPixel(c, 7, 8, Paint{UseIndex: true, Index: 6}, nil, false)
	f2, _ := e.AddFrame(f, 160)
	linked, _ := e.CopyCel(c, l, f2, true)
	_, _ = e.SetTag(Tag{Name: "idle", From: f, To: f2, Direction: Reverse})
	done(t, e)
	raw, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	other, err := Decode(raw, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Info(), other.Info()) {
		t.Fatal("metadata changed in round-trip")
	}
	if hash(t, d, f) != hash(t, other, f) {
		t.Fatal("pixels changed")
	}
	e = edit(t, other)
	_ = e.SetPixel(c, 7, 8, Paint{UseIndex: true, Index: 8}, nil, false)
	done(t, e)
	pixel(t, other, linked, 7, 8, other.Info().Palette[8].Color)
	again, err := d.Encode()
	if err != nil || !bytes.Equal(raw, again) {
		t.Fatal("encoding is not deterministic", err)
	}
}
func rezip(t *testing.T, raw []byte, fn func(string, []byte) (string, []byte)) []byte {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, f := range r.File {
		v, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(v)
		v.Close()
		if err != nil {
			t.Fatal(err)
		}
		name, data := fn(f.Name, data)
		dst, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = dst.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestSourceRejectsChecksumTraversalAndLimits(t *testing.T) {
	d, _, _, c := doc(t, RGBA)
	e := edit(t, d)
	_ = e.SetPixel(c, 1, 1, Paint{Color: red}, nil, false)
	done(t, e)
	raw, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	corrupt := rezip(t, raw, func(n string, b []byte) (string, []byte) {
		if n != "manifest.json" {
			b[len(b)-1] ^= 1
		}
		return n, b
	})
	if _, err = Decode(corrupt, Limits{}); err == nil {
		t.Fatal("bad checksum accepted")
	}
	traversal := rezip(t, raw, func(n string, b []byte) (string, []byte) {
		if n != "manifest.json" {
			return "../../outside", b
		}
		return n, b
	})
	if _, err = Decode(traversal, Limits{}); err == nil {
		t.Fatal("traversal accepted")
	}
	huge := rezip(t, raw, func(n string, b []byte) (string, []byte) {
		if n == "manifest.json" {
			var m manifest
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatal(err)
			}
			m.Document.Canvas.Width = 1 << 30
			b, _ = json.Marshal(m)
		}
		return n, b
	})
	if _, err = Decode(huge, Limits{}); err == nil {
		t.Fatal("huge canvas accepted")
	}
	if _, err = Decode(raw, Limits{MaxFileBytes: 10}); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
}
func TestPNGAndSheetExport(t *testing.T) {
	d, _, f, c := doc(t, RGBA)
	e := edit(t, d)
	_ = e.SetPixel(c, 1, 2, Paint{Color: Color{R: 55, G: 66, B: 77, A: 128}}, nil, false)
	_, _ = e.DuplicateFrame(f, false)
	done(t, e)
	data, err := d.PNG(f, Albedo)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := ImportPNG(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	pixel(t, loaded, loaded.Info().Cels[0].ID, 1, 2, Color{55, 66, 77, 128})
	sheet, err := d.ExportSheet(2, Albedo)
	if err != nil {
		t.Fatal(err)
	}
	if sheet.Width != 32 || sheet.Height != 16 || sheet.Frames[0].ID != f {
		t.Fatal(sheet)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(sheet.PNG))
	if err != nil || cfg.Width != 32 {
		t.Fatal(cfg, err)
	}
}
func TestRegionCompositeMatchesFull(t *testing.T) {
	d, _, f, c := doc(t, RGBA)
	retained, err := d.Render(f, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 8; n++ {
		e := edit(t, d)
		_ = e.SetPixel(c, n, 5, Paint{Color: red}, nil, false)
		ch := done(t, e)
		if err = d.RenderRegion(f, retained, ch.DirtyRect(), RenderOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	full, err := d.Render(f, RenderOptions{})
	if err != nil || !bytes.Equal(full.Pix, retained.Pix) {
		t.Fatal("incremental composite differs", err)
	}
}
func TestProposalIsAtomicAndCancellation(t *testing.T) {
	d, l, f, c := doc(t, RGBA)
	rev := d.Revision()
	ops := []Operation{{Kind: "stroke", Cel: c, Points: []image.Point{{1, 1}}, Brush: brush(red)}, {Kind: "duration", Frame: f, DurationMS: -1}}
	if _, _, err := d.Propose(context.Background(), rev, "agent", ops); err == nil {
		t.Fatal("invalid proposal accepted")
	}
	pixel(t, d, c, 1, 1, Color{})
	if d.Revision() != rev {
		t.Fatal("partial proposal published")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := d.Propose(ctx, rev, "agent", []Operation{{Kind: "add-layer", LayerKind: Raster, Name: "new", Parent: l}}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestConcurrentReadAndPreparedWrite(t *testing.T) {
	d, _, f, c := doc(t, RGBA)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				_ = d.Info()
				if _, err := d.Render(f, RenderOptions{}); err != nil {
					t.Error(err)
				}
				if _, err := d.Encode(); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	for i := 0; i < 30; i++ {
		e := edit(t, d)
		_ = e.SetPixel(c, i%16, i/16, Paint{Color: red}, nil, false)
		done(t, e)
	}
	wg.Wait()
}
func TestJournalBudgetRejectsBeforePublish(t *testing.T) {
	d, _, _, c := doc(t, RGBA)
	e := edit(t, d)
	_ = e.SetPixel(c, 1, 1, Paint{Color: red}, nil, false)
	ch, err := e.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	j := NewJournal(Limits{MaxHistoryBytes: 1})
	if err = j.Exec(ch); err == nil {
		t.Fatal("budget ignored")
	}
	pixel(t, d, c, 1, 1, Color{})
}
func TestSymmetryAndPaletteGradient(t *testing.T) {
	d, _, _, c := doc(t, Indexed)
	e := edit(t, d)
	b := Brush{Size: 1, MirrorX: true, MirrorY: true, Paint: Paint{UseIndex: true, Index: 6}}
	g, err := e.Stroke(c, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = g.Point(image.Pt(2, 3)); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	for _, p := range []image.Point{{2, 3}, {13, 3}, {2, 12}, {13, 12}} {
		pixel(t, d, c, p.X, p.Y, d.Info().Palette[6].Color)
	}
	e = edit(t, d)
	if err = e.Gradient(context.Background(), c, image.Pt(0, 0), image.Pt(15, 0), Paint{Index: 6, UseIndex: true}, Paint{Index: 9, UseIndex: true}, nil, false); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	pixel(t, d, c, 0, 0, d.Info().Palette[6].Color)
	pixel(t, d, c, 15, 0, d.Info().Palette[9].Color)
}
func FuzzDecode(f *testing.F) {
	d, err := New(4, 4, RGBA, Limits{})
	if err != nil {
		f.Fatal(err)
	}
	data, err := d.Encode()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Add([]byte("PK\x03\x04"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		_, _ = Decode(data, Limits{MaxFileBytes: 1 << 20, MaxPixelBytes: 1 << 20, MaxCels: 64, MaxFrames: 64, MaxLayers: 32})
	})
}
func BenchmarkStroke256(b *testing.B) {
	d, err := New(256, 256, RGBA, Limits{})
	if err != nil {
		b.Fatal(err)
	}
	c := d.Info().Cels[0].ID
	frame := d.Info().Frames[0].ID
	im, _ := d.Render(frame, RenderOptions{})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := edit(b, d)
		br := brush(red)
		if i%2 == 1 {
			br = brush(blue)
		}
		if err = e.Line(c, image.Pt(0, 128), image.Pt(255, 128), br, nil); err != nil {
			b.Fatal(err)
		}
		ch := done(b, e)
		if ch != nil {
			if err = d.RenderRegion(frame, im, ch.DirtyRect(), RenderOptions{}); err != nil {
				b.Fatal(err)
			}
		}
	}
}
