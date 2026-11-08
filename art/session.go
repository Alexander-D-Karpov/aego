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
	Channel         Channel
	Selection       *Selection
	Clipboard       *Clipboard
	saved           Checkpoint
	draft           *Edit
	gesture         *Gesture
	tool            string
	start           image.Point
	brush           Brush
	filled          bool
	gestureRevision uint64
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
	return SessionInfo{s.Document.Info(), s.Layer, s.Frame, s.Channel, u, r, s.Document.DirtySince(s.saved), s.draft != nil, s.Selection.Bounds(), s.Selection.Count()}
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
	if tool != "pencil" && tool != "eraser" && tool != "line" && tool != "rectangle" && tool != "ellipse" && tool != "gradient" {
		return fmt.Errorf("art: unsupported pointer tool")
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
	} else {
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
func (s *Session) EndGesture() error {
	if s.draft == nil {
		return nil
	}
	e := s.draft
	s.draft = nil
	s.gesture = nil
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
	case "add-layer", "duplicate-layer":
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
	for _, l := range i.Layers {
		if l.ID == s.Layer {
			found = true
		}
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
	o := RenderOptions{Channel: s.Channel, References: references}
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
