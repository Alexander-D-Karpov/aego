package art

import (
	"context"
	"fmt"
	"image"
)

// HistoryHost makes the same interaction controller usable with the editor's
// history.History or the standalone Journal. There is never a second active
// undo stack for a native document.
type HistoryHost interface {
	Exec(*Change) error
	Undo() error
	Redo() error
	Counts() (int, int)
}
type Session struct {
	Document        *Document
	History         HistoryHost
	Layer, Frame    ID
	IsolatedLayer   ID
	Channel         Channel
	Selection       *Selection
	Clipboard       *Clipboard
	customBrush     *Clipboard
	brushRevision   uint64
	saved           Checkpoint
	draft           *Edit
	gesture         *Gesture
	tool            string
	start           image.Point
	brush           Brush
	filled          bool
	gestureRevision uint64
	shapePoints     int
	preview         previewCache
}
type SessionInfo struct {
	Document       Info            `json:"document"`
	Layer          ID              `json:"layer"`
	Frame          ID              `json:"frame"`
	Channel        Channel         `json:"channel"`
	Undo           int             `json:"undo"`
	Redo           int             `json:"redo"`
	Dirty          bool            `json:"dirty"`
	Gesture        bool            `json:"gesture"`
	Selection      image.Rectangle `json:"selection"`
	SelectedPixels int             `json:"selectedPixels"`
	BrushWidth     int             `json:"brushWidth,omitempty"`
	BrushHeight    int             `json:"brushHeight,omitempty"`
	Isolation      ID              `json:"isolation,omitempty"`
}

func NewSession(d *Document, h HistoryHost) (*Session, error) {
	if d == nil || h == nil {
		return nil, fmt.Errorf("art: document and history host are required")
	}
	i := d.Info()
	saved, err := d.Checkpoint(i.Revision)
	if err != nil {
		return nil, err
	}
	return &Session{Document: d, History: h, Layer: i.Layers[0].ID, Frame: i.Frames[0].ID, Channel: Albedo, saved: saved}, nil
}
func (s *Session) Info() SessionInfo {
	u, r := s.History.Counts()
	b := s.BrushSize()
	return SessionInfo{Document: s.Document.Info(), Layer: s.Layer, Frame: s.Frame, Channel: s.Channel, Undo: u, Redo: r, Dirty: s.Document.DirtySince(s.saved), Gesture: s.draft != nil, Selection: s.Selection.Bounds(), SelectedPixels: s.Selection.Count(), BrushWidth: b.X, BrushHeight: b.Y, Isolation: s.IsolatedLayer}
}
func (s *Session) MarkSaved(expected uint64) error {
	cp, err := s.Document.Checkpoint(expected)
	if err != nil {
		return err
	}
	s.saved = cp
	return nil
}

// MarkUnsaved marks a recovered/new source as needing an explicit save.
func (s *Session) MarkUnsaved() { s.saved = Checkpoint{} }

func (s *Session) ActiveCel() ID {
	i := s.Document.Info()
	for _, c := range i.Cels {
		if c.Layer == s.Layer && c.Frame == s.Frame && c.Channel == s.Channel {
			return c.ID
		}
	}
	return ""
}
func (s *Session) Select(layer, frame ID, ch Channel) error {
	if s.draft != nil {
		return fmt.Errorf("art: finish or cancel the current gesture")
	}
	i := s.Document.Info()
	if layer != "" {
		ok := false
		for _, v := range i.Layers {
			if v.ID == layer {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("art: missing layer")
		}
	}
	if frame != "" {
		ok := false
		for _, v := range i.Frames {
			if v.ID == frame {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("art: missing frame")
		}
	}
	if ch != "" && !validChannel(ch) {
		return fmt.Errorf("art: invalid channel")
	}
	if layer != "" {
		s.Layer = layer
	}
	if frame != "" {
		s.Frame = frame
	}
	if ch != "" {
		s.Channel = ch
	}
	return nil
}
func (s *Session) BeginGesture(tool string, p image.Point, b Brush, filled bool) error {
	if s.draft != nil {
		return fmt.Errorf("art: a gesture is already active")
	}
	if tool != "pencil" && tool != "eraser" && tool != "line" && tool != "rectangle" && tool != "ellipse" && tool != "gradient" && tool != "curve" && tool != "polygon" {
		return fmt.Errorf("art: unsupported pointer tool")
	}
	var err error
	b, err = s.resolveBrush(b)
	if err != nil {
		return err
	}
	e, err := s.Document.Begin(s.Document.Revision(), tool)
	if err != nil {
		return err
	}
	s.draft = e
	s.gestureRevision = e.revision
	s.start = p
	s.brush = b
	s.filled = filled
	s.tool = tool
	s.shapePoints = 0
	cel, err := e.EnsureCel(s.Layer, s.Frame, s.Channel)
	if err != nil {
		s.CancelGesture()
		return err
	}
	if tool == "pencil" || tool == "eraser" {
		b.Erase = tool == "eraser"
		s.gesture, err = e.Stroke(cel, b, s.Selection)
		if err == nil {
			err = s.gesture.Point(p)
		}
	} else if tool != "curve" && tool != "polygon" {
		err = s.previewShape(p)
	}
	if err != nil {
		s.CancelGesture()
	}
	return err
}
func (s *Session) previewShape(p image.Point) error {
	if s.draft != nil {
		s.draft.Cancel()
	}
	e, err := s.Document.Begin(s.gestureRevision, s.tool)
	if err != nil {
		return err
	}
	s.draft = e
	cel, err := e.EnsureCel(s.Layer, s.Frame, s.Channel)
	if err != nil {
		return err
	}
	if s.tool == "gradient" {
		return e.Gradient(context.Background(), cel, s.start, p, s.brush.Paint, s.brush.Background, s.Selection, s.brush.Dither > 0)
	}
	return e.Shape(cel, s.tool, s.start, p, s.brush, s.Selection, s.filled)
}
func (s *Session) MoveGesture(points []image.Point) error {
	if s.draft == nil {
		return fmt.Errorf("art: no active gesture")
	}
	if len(points) > 65536 {
		return ErrLimit
	}
	if len(points) == 0 {
		return nil
	}
	if s.tool == "curve" || s.tool == "polygon" {
		return s.PreviewGesturePoints(points)
	}
	var err error
	if s.gesture != nil {
		for _, p := range points {
			if err = s.gesture.Point(p); err != nil {
				break
			}
		}
	} else {
		err = s.previewShape(points[len(points)-1])
	}
	if err != nil {
		s.CancelGesture()
	}
	return err
}

// PreviewGesturePoints replaces an uncommitted multi-point shape using the
// same drawing operations and revision as the eventual history command. The
// host supplies the complete cubic curve (four points) or polygon (at least
// three points), rather than committing each pointer movement separately.
func (s *Session) PreviewGesturePoints(points []image.Point) error {
	if s.draft == nil || (s.tool != "curve" && s.tool != "polygon") {
		return fmt.Errorf("art: no active multi-point shape")
	}
	if (s.tool == "curve" && len(points) != 4) || (s.tool == "polygon" && len(points) < 3) {
		return fmt.Errorf("art: incomplete shape points")
	}
	if len(points) > 65536 {
		return ErrLimit
	}
	s.draft.Cancel()
	e, err := s.Document.Begin(s.gestureRevision, s.tool)
	if err != nil {
		s.CancelGesture()
		return err
	}
	s.draft = e
	cel, err := e.EnsureCel(s.Layer, s.Frame, s.Channel)
	if err == nil {
		if s.tool == "curve" {
			err = e.Curve(cel, points[0], points[1], points[2], points[3], s.brush, s.Selection)
		} else {
			err = e.Polygon(cel, points, s.brush, s.Selection, s.filled)
		}
	}
	if err != nil {
		s.CancelGesture()
		return err
	}
	s.shapePoints = len(points)
	return nil
}

func (s *Session) EndGesture() error {
	if s.draft == nil {
		return nil
	}
	if (s.tool == "curve" || s.tool == "polygon") && s.shapePoints == 0 {
		s.CancelGesture()
		return fmt.Errorf("art: incomplete shape points")
	}
	e := s.draft
	s.draft = nil
	s.gesture = nil
	s.shapePoints = 0
	c, err := e.Prepare()
	if err != nil {
		e.Cancel()
		return err
	}
	return s.History.Exec(c)
}
func (s *Session) CancelGesture() {
	if s.draft != nil {
		s.draft.Cancel()
	}
	s.draft = nil
	s.gesture = nil
	s.shapePoints = 0
}

// SamplePaint samples the selected layer, frame and channel, not onion skin,
// reference overlays or the other composited layers.
func (s *Session) SamplePaint(p image.Point) (Paint, error) {
	if s.draft != nil {
		return Paint{}, fmt.Errorf("art: finish or cancel the current gesture")
	}
	return s.Document.SamplePaint(s.ActiveCel(), s.Channel, p.X, p.Y)
}
func (s *Session) Execute(ctx context.Context, op Operation) (ID, error) {
	if s.draft != nil {
		return "", fmt.Errorf("art: a gesture is active")
	}
	if op.Layer == "" {
		op.Layer = s.Layer
	}
	if op.Frame == "" {
		op.Frame = s.Frame
	}
	if op.Channel == "" {
		op.Channel = s.Channel
	}
	switch op.Kind {
	case "stroke", "fill", "line", "rectangle", "ellipse", "gradient", "polygon", "curve":
		var err error
		op.Brush, err = s.resolveBrush(op.Brush)
		if err != nil {
			return "", err
		}
	default:
		if op.Cel == "" {
			op.Cel = s.ActiveCel()
		}
	}
	e, err := s.Document.Begin(s.Document.Revision(), op.Kind)
	if err != nil {
		return "", err
	}
	id, err := e.Execute(ctx, op, s.Selection)
	if err != nil {
		e.Cancel()
		return "", err
	}
	c, err := e.Prepare()
	if err != nil {
		e.Cancel()
		return "", err
	}
	if err = s.History.Exec(c); err != nil {
		return "", err
	}
	switch op.Kind {
	case "add-layer", "duplicate-layer", "merge-down":
		s.Layer = id
	case "add-frame", "duplicate-frame":
		s.Frame = id
	}
	s.repairSelection()
	return id, nil
}
func (s *Session) repairSelection() {
	i := s.Document.Info()
	found := false
	isolated := false
	for _, l := range i.Layers {
		if l.ID == s.Layer {
			found = true
		}
		if l.ID == s.IsolatedLayer {
			isolated = true
		}
	}
	if !isolated {
		s.IsolatedLayer = ""
	}
	if !found {
		s.Layer = i.Layers[0].ID
	}
	found = false
	for _, f := range i.Frames {
		if f.ID == s.Frame {
			found = true
		}
	}
	if !found {
		s.Frame = i.Frames[0].ID
	}
}

// SetIsolation is a view operation; it does not change visibility, revisions,
// exports or the document's undo stack.
func (s *Session) SetIsolation(layer ID) error {
	if layer != "" {
		found := false
		for _, candidate := range s.Document.Info().Layers {
			if candidate.ID == layer {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("art: missing isolated layer")
		}
	}
	s.IsolatedLayer = layer
	return nil
}
func (s *Session) Undo() error {
	if s.draft != nil {
		s.CancelGesture()
		return nil
	}
	if err := s.History.Undo(); err != nil {
		return err
	}
	s.repairSelection()
	return nil
}
func (s *Session) Redo() error {
	if s.draft != nil {
		return fmt.Errorf("art: a gesture is active")
	}
	if err := s.History.Redo(); err != nil {
		return err
	}
	s.repairSelection()
	return nil
}
func (s *Session) SetSelection(kind string, points []image.Point, tolerance int) error {
	if s.draft != nil {
		return fmt.Errorf("art: a gesture is active")
	}
	i := s.Document.Info()
	var selection *Selection
	var err error
	switch kind {
	case "none":
		s.Selection = nil
		return nil
	case "all":
		selection, err = RectSelection(i.Canvas.Width, i.Canvas.Height, image.Rect(0, 0, i.Canvas.Width, i.Canvas.Height), false)
	case "rectangle", "ellipse":
		if len(points) != 2 {
			return fmt.Errorf("art: selection needs two points")
		}
		a, b := points[0], points[1]
		r := image.Rect(min(a.X, b.X), min(a.Y, b.Y), max(a.X, b.X)+1, max(a.Y, b.Y)+1)
		selection, err = RectSelection(i.Canvas.Width, i.Canvas.Height, r, kind == "ellipse")
	case "lasso":
		selection, err = LassoSelection(i.Canvas.Width, i.Canvas.Height, points)
	case "wand":
		if len(points) != 1 {
			return fmt.Errorf("art: wand needs one point")
		}
		selection, err = s.Document.Wand(s.ActiveCel(), points[0].X, points[0].Y, tolerance, true)
	default:
		return fmt.Errorf("art: unsupported selection")
	}
	if err != nil {
		return err
	}
	s.Selection = selection
	return nil
}
func (s *Session) Copy() error {
	clip, err := s.Document.Copy(s.ActiveCel(), s.Selection)
	if err != nil {
		return err
	}
	s.Clipboard = clip
	return nil
}

// CaptureBrush uses the existing cel/selection copier. Tool state is local to
// this session and never makes the document dirty or adds an Undo entry.
func (s *Session) CaptureBrush() error {
	if s.draft != nil {
		return fmt.Errorf("art: finish or cancel the current gesture")
	}
	if s.Selection == nil || s.Selection.Count() == 0 {
		return fmt.Errorf("art: select a brush region first")
	}
	r := s.Selection.Bounds()
	if r.Dx() > 128 || r.Dy() > 128 {
		return fmt.Errorf("art: custom brushes are limited to 128 by 128 pixels")
	}
	clip, err := s.Document.Copy(s.ActiveCel(), s.Selection)
	if err != nil {
		return err
	}
	s.customBrush = clip
	s.brushRevision++
	return nil
}
func (s *Session) ClearBrush() {
	s.customBrush = nil
	s.brushRevision++
}

// BrushRevision changes when a captured custom brush changes, without marking
// document pixels dirty. Presentation caches can use it to invalidate samples.
func (s *Session) BrushRevision() uint64 { return s.brushRevision }

// PreviewBrush resolves a detached copy using the same rules as painting. The
// caller may render it in a scratch document without exposing session-owned data.
func (s *Session) PreviewBrush(b Brush) (Brush, error) {
	b, err := s.resolveBrush(b)
	if err != nil || b.Pattern == nil {
		return b, err
	}
	clip := *b.Pattern
	clip.Pixels = append([]Color(nil), clip.Pixels...)
	clip.Mask = append([]bool(nil), clip.Mask...)
	b.Pattern = &clip
	return b, nil
}
func (s *Session) BrushSize() image.Point {
	if s.customBrush == nil {
		return image.Point{}
	}
	return image.Pt(s.customBrush.Width, s.customBrush.Height)
}
func (s *Session) resolveBrush(b Brush) (Brush, error) {
	if b.Source != "stamp" && b.Source != "pattern" {
		return b, nil
	}
	if s.customBrush == nil {
		return b, fmt.Errorf("art: capture a brush from a selection first")
	}
	b.Pattern = s.customBrush
	b.Stamp = b.Source == "stamp"
	if b.Stamp {
		b.Size = max(b.Pattern.Width, b.Pattern.Height)
		b.Round = false
		b.PixelPerfect = false
	}
	return b, nil
}
func (s *Session) Paste(at image.Point) error {
	if s.draft != nil {
		return fmt.Errorf("art: a gesture is active")
	}
	if s.Clipboard == nil {
		return fmt.Errorf("art: clipboard is empty")
	}
	e, err := s.Document.Begin(s.Document.Revision(), "Paste")
	if err != nil {
		return err
	}
	cel, err := e.EnsureCel(s.Layer, s.Frame, s.Channel)
	if err != nil {
		e.Cancel()
		return err
	}
	if err = e.Paste(cel, s.Clipboard, at, s.Selection); err != nil {
		e.Cancel()
		return err
	}
	ch, err := e.Prepare()
	if err != nil {
		e.Cancel()
		return err
	}
	return s.History.Exec(ch)
}
func (s *Session) Preview(frame ID, onion bool, references bool) (*image.NRGBA, error) {
	if frame == "" {
		frame = s.Frame
	}
	o := RenderOptions{Channel: s.Channel, References: references, Isolate: s.IsolatedLayer}
	im, err := s.previewImage(frame, o)
	if err != nil || !onion {
		return im, err
	}
	ghost, err := s.Document.Onion(frame, 1, 1, o)
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(im.Pix); i += 4 {
		v := blendPixel(Color{ghost.Pix[i], ghost.Pix[i+1], ghost.Pix[i+2], ghost.Pix[i+3]}, Color{im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3]}, 255, BlendNormal)
		im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = v.R, v.G, v.B, v.A
	}
	return im, nil
}
