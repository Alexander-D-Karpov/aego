package art

import (
	"fmt"
	"image"
	"reflect"
)

// Edit is an unpublished, private COW transaction. A caller must not share an
// Edit between goroutines. Document and Change entrypoints are synchronized.
type Edit struct {
	doc        *Document
	base       *state
	work       *state
	revision   uint64
	label      string
	done       bool
	mutable    map[ID]map[tileKey]bool
	dirty      image.Rectangle
	structural bool
	allocated  int64
	lastCelID  ID
	lastCel    int
}

func (d *Document) Begin(expected uint64, label string) (*Edit, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, ErrClosed
	}
	if expected != d.revision {
		return nil, ErrConflict
	}
	return &Edit{doc: d, base: d.current, work: d.current.clone(), revision: expected, label: label, mutable: make(map[ID]map[tileKey]bool)}, nil
}
func (e *Edit) check() error {
	if e == nil || e.done {
		return ErrDone
	}
	return nil
}
func (e *Edit) Info() Info { return info(e.doc.id, e.revision, e.work) }
func (e *Edit) Cancel() {
	if e != nil {
		e.done = true
		e.work = nil
		e.mutable = nil
	}
}
func (e *Edit) DirtyRect() image.Rectangle { return e.dirty }
func (e *Edit) touch(r image.Rectangle) {
	e.dirty = e.dirty.Union(r.Intersect(image.Rect(0, 0, e.work.Canvas.Width, e.work.Canvas.Height)))
}
// celIndex caches only a checked slot within this draft, never cel content.
func (e *Edit) celIndex(id ID) (int, bool) {
	if id == e.lastCelID && e.lastCel >= 0 && e.lastCel < len(e.work.Cels) && e.work.Cels[e.lastCel].ID == id {
		return e.lastCel, true
	}
	i, ok := e.work.cel(id)
	if ok {
		e.lastCelID, e.lastCel = id, i
	}
	return i, ok
}

func (e *Edit) raw(im ID, x, y int, v uint32) error {
	p := e.work.Images[im]
	if p == nil {
		return fmt.Errorf("art: missing image")
	}
	if x < 0 || y < 0 || x >= p.Width || y >= p.Height {
		return nil
	}
	if p.at(x, y) == v {
		return nil
	}
	m := e.mutable[im]
	if m == nil {
		p = p.clone()
		e.work.Images[im] = p
		m = make(map[tileKey]bool)
		e.mutable[im] = m
	}
	key := tileKey{x / TileSize, y / TileSize}
	if !m[key] {
		t, err := e.allocateTile(p.Tiles[key])
		if err != nil {
			return err
		}
		p.Tiles[key] = t
		m[key] = true
	}
	p.Tiles[key][(y%TileSize)*TileSize+x%TileSize] = v
	for _, c := range e.work.Cels {
		if c.Image == im {
			e.touch(image.Rect(x+c.X, y+c.Y, x+c.X+1, y+c.Y+1))
		}
	}
	return nil
}

// Bound draft allocation before publication as well as validating the final
// canonical state in Prepare. A large proposal cannot allocate an unlimited
// number of tiles and rely on a late validation error for its only guard.
func (e *Edit) allocateTile(source *tile) (*tile, error) {
	const bytes = int64(TileSize * TileSize * 4)
	if e.allocated > e.doc.limits.MaxPixelBytes-bytes {
		return nil, ErrLimit
	}
	e.allocated += bytes
	block := new(tile)
	if source != nil {
		*block = *source
	}
	return block, nil
}

// expandCel makes a cropped imported image editable anywhere on the canvas.
// The image identity is preserved, so linked cels continue to share pixels.
// Their origins are shifted together; independent copies keep their old image.
func (e *Edit) expandCel(c Cel) (Cel, error) {
	p := e.work.Images[c.Image]
	r := image.Rect(0, 0, p.Width, p.Height).Union(
		image.Rect(-c.X, -c.Y, e.work.Canvas.Width-c.X, e.work.Canvas.Height-c.Y))
	if r.Dx() > e.doc.limits.MaxDimension || r.Dy() > e.doc.limits.MaxDimension ||
		int64(r.Dx())*int64(r.Dy()) > int64(e.doc.limits.MaxCanvasPixels) {
		return Cel{}, ErrLimit
	}
	q := newPixels(r.Dx(), r.Dy())
	mutable := make(map[tileKey]bool)
	for key, block := range p.Tiles {
		for y := 0; y < TileSize; y++ {
			for x := 0; x < TileSize; x++ {
				v := block[y*TileSize+x]
				sx, sy := key.X*TileSize+x, key.Y*TileSize+y
				if v == 0 || sx >= p.Width || sy >= p.Height {
					continue
				}
				dx, dy := sx-r.Min.X, sy-r.Min.Y
				dk := tileKey{dx / TileSize, dy / TileSize}
				if q.Tiles[dk] == nil {
					block, err := e.allocateTile(nil)
					if err != nil {
						return Cel{}, err
					}
					q.Tiles[dk] = block
					mutable[dk] = true
				}
				q.Tiles[dk][dy%TileSize*TileSize+dx%TileSize] = v
			}
		}
	}
	e.work.Images[c.Image] = q
	e.mutable[c.Image] = mutable
	for i := range e.work.Cels {
		linked := &e.work.Cels[i]
		if linked.Image == c.Image {
			linked.X += r.Min.X
			linked.Y += r.Min.Y
			if linked.ID == c.ID {
				c = *linked
			}
		}
	}
	return c, nil
}

func (e *Edit) celForWrite(id ID) (Cel, error) {
	if err := e.check(); err != nil {
		return Cel{}, err
	}
	i, ok := e.celIndex(id)
	if !ok {
		return Cel{}, fmt.Errorf("art: missing cel")
	}
	c := e.work.Cels[i]
	if e.work.isLocked(c.Layer) {
		return Cel{}, ErrLocked
	}
	return c, nil
}
func (e *Edit) AddLayer(name string, kind LayerKind, parent ID) (ID, error) {
	if err := e.check(); err != nil {
		return "", err
	}
	if len(e.work.Layers) >= e.doc.limits.MaxLayers {
		return "", ErrLimit
	}
	if kind != Raster && kind != Group && kind != Reference {
		return "", fmt.Errorf("art: invalid layer kind")
	}
	if parent != "" {
		i, ok := e.work.layer(parent)
		if !ok || e.work.Layers[i].Kind != Group {
			return "", fmt.Errorf("art: invalid group")
		}
		if e.work.isLocked(parent) {
			return "", ErrLocked
		}
	}
	id := e.doc.newID()
	e.work.Layers = append(e.work.Layers, Layer{ID: id, Parent: parent, Name: name, Kind: kind, Visible: true, Opacity: 255, Blend: BlendNormal})
	e.structural = true
	return id, nil
}
func (e *Edit) SetLayer(v Layer) error {
	if err := e.check(); err != nil {
		return err
	}
	i, ok := e.work.layer(v.ID)
	if !ok {
		return fmt.Errorf("art: missing layer")
	}
	old := e.work.Layers[i]
	if v.Kind != old.Kind {
		return fmt.Errorf("art: layer kind cannot be changed")
	}
	// Unlock is an explicit structural edit; it must still respect a locked group.
	if old.Parent != "" && e.work.isLocked(old.Parent) {
		return ErrLocked
	}
	e.work.Layers[i] = v
	e.structural = true
	return nil
}
func (e *Edit) MoveLayer(id ID, offset int) error {
	if err := e.check(); err != nil {
		return err
	}
	i, ok := e.work.layer(id)
	if !ok {
		return fmt.Errorf("art: missing layer")
	}
	if e.work.isLocked(id) {
		return ErrLocked
	}
	j := i + offset
	if j < 0 || j >= len(e.work.Layers) {
		return nil
	}
	if e.work.Layers[i].Parent != e.work.Layers[j].Parent {
		return fmt.Errorf("art: reorder must stay within a group")
	}
	v := e.work.Layers[i]
	if j < i {
		copy(e.work.Layers[j+1:i+1], e.work.Layers[j:i])
	} else {
		copy(e.work.Layers[i:j], e.work.Layers[i+1:j+1])
	}
	e.work.Layers[j] = v
	e.structural = true
	return nil
}
func (e *Edit) DeleteLayer(id ID) error {
	if err := e.check(); err != nil {
		return err
	}
	if e.work.isLocked(id) {
		return ErrLocked
	}
	gone := map[ID]bool{id: true}
	for changed := true; changed; {
		changed = false
		for _, l := range e.work.Layers {
			if gone[l.Parent] && !gone[l.ID] {
				gone[l.ID] = true
				changed = true
			}
		}
	}
	var layers []Layer
	for _, l := range e.work.Layers {
		if !gone[l.ID] {
			layers = append(layers, l)
		}
	}
	if len(layers) == 0 {
		return fmt.Errorf("art: cannot delete the last layer")
	}
	var cels []Cel
	for _, c := range e.work.Cels {
		if !gone[c.Layer] {
			cels = append(cels, c)
		}
	}
	e.work.Layers, e.work.Cels = layers, cels
	e.structural = true
	return nil
}
func (e *Edit) AddFrame(after ID, duration int) (ID, error) {
	if err := e.check(); err != nil {
		return "", err
	}
	if len(e.work.Frames) >= e.doc.limits.MaxFrames {
		return "", ErrLimit
	}
	i := -1
	if after != "" {
		var ok bool
		i, ok = e.work.frame(after)
		if !ok {
			return "", fmt.Errorf("art: missing frame")
		}
	}
	if duration < 1 || duration > 600000 {
		return "", fmt.Errorf("art: invalid frame duration")
	}
	id := e.doc.newID()
	e.work.Frames = append(e.work.Frames, Frame{})
	copy(e.work.Frames[i+2:], e.work.Frames[i+1:])
	e.work.Frames[i+1] = Frame{id, duration}
	e.structural = true
	return id, nil
}
func (e *Edit) SetDuration(frame ID, ms int) error {
	if err := e.check(); err != nil {
		return err
	}
	i, ok := e.work.frame(frame)
	if !ok || ms < 1 || ms > 600000 {
		return fmt.Errorf("art: invalid frame duration")
	}
	e.work.Frames[i].DurationMS = ms
	e.structural = true
	return nil
}
func (e *Edit) DeleteFrame(id ID) error {
	if err := e.check(); err != nil {
		return err
	}
	i, ok := e.work.frame(id)
	if !ok {
		return fmt.Errorf("art: missing frame")
	}
	if len(e.work.Frames) == 1 {
		return fmt.Errorf("art: cannot delete the last frame")
	}
	// A tag endpoint is a stable frame ID, not a numeric offset. Retarget it to
	// the nearest surviving frame; remove a tag that only covered this frame.
	var tags []Tag
	for _, t := range e.work.Tags {
		if t.From == id && t.To == id {
			continue
		}
		if t.From == id {
			t.From = e.work.Frames[i+1].ID
		}
		if t.To == id {
			t.To = e.work.Frames[i-1].ID
		}
		tags = append(tags, t)
	}
	e.work.Tags = tags
	e.work.Frames = append(e.work.Frames[:i], e.work.Frames[i+1:]...)
	var cels []Cel
	for _, c := range e.work.Cels {
		if c.Frame != id {
			cels = append(cels, c)
		}
	}
	e.work.Cels = cels
	var slices []Slice
	for _, s := range e.work.Slices {
		if s.Frame != id {
			slices = append(slices, s)
		}
	}
	e.work.Slices = slices
	e.structural = true
	return nil
}
func (e *Edit) EnsureCel(layer, frame ID, ch Channel) (ID, error) {
	if err := e.check(); err != nil {
		return "", err
	}
	if i, ok := e.work.findCel(layer, frame, ch); ok {
		return e.work.Cels[i].ID, nil
	}
	li, ok := e.work.layer(layer)
	if !ok || e.work.Layers[li].Kind == Group {
		return "", fmt.Errorf("art: invalid cel layer")
	}
	if e.work.isLocked(layer) {
		return "", ErrLocked
	}
	if _, ok = e.work.frame(frame); !ok || !validChannel(ch) {
		return "", fmt.Errorf("art: invalid cel frame/channel")
	}
	if len(e.work.Cels) >= e.doc.limits.MaxCels {
		return "", ErrLimit
	}
	im, id := e.doc.newID(), e.doc.newID()
	e.work.Images[im] = newPixels(e.work.Canvas.Width, e.work.Canvas.Height)
	e.work.Cels = append(e.work.Cels, Cel{ID: id, Layer: layer, Frame: frame, Image: im, Channel: ch, Opacity: 255})
	e.structural = true
	return id, nil
}

// shareIndependent freezes any draft-owned tiles before sharing them with an
// independent image. Without revoking ownership, a subsequent write to the
// original in this same transaction would silently change its new copy too.
func (e *Edit) shareIndependent(source, dest ID) {
	p := e.work.Images[source]
	e.work.Images[dest] = p
	if owned := e.mutable[source]; owned != nil {
		e.work.Images[source] = p.clone()
		for key := range owned {
			owned[key] = false
		}
	}
}

// CopyCel replaces the destination cel. A normal copy gets a distinct storage
// identity but shares immutable tiles until a write; a linked copy shares ID.
func (e *Edit) CopyCel(source, layer, frame ID, linked bool) (ID, error) {
	c, err := e.celForWrite(source)
	if err != nil {
		return "", err
	}
	if e.work.isLocked(layer) {
		return "", ErrLocked
	}
	dst, err := e.EnsureCel(layer, frame, c.Channel)
	if err != nil {
		return "", err
	}
	di, _ := e.work.cel(dst)
	if dst == source {
		return dst, nil
	}
	im := c.Image
	if !linked {
		im = e.doc.newID()
		e.shareIndependent(c.Image, im)
	}
	e.work.Cels[di].Image = im
	e.work.Cels[di].X = c.X
	e.work.Cels[di].Y = c.Y
	e.work.Cels[di].Opacity = c.Opacity
	e.structural = true
	return dst, nil
}
func (e *Edit) Unlink(cel ID) error {
	c, err := e.celForWrite(cel)
	if err != nil {
		return err
	}
	n := 0
	for _, v := range e.work.Cels {
		if v.Image == c.Image {
			n++
		}
	}
	if n < 2 {
		return nil
	}
	im := e.doc.newID()
	e.shareIndependent(c.Image, im)
	i, _ := e.work.cel(cel)
	e.work.Cels[i].Image = im
	e.structural = true
	return nil
}
func (e *Edit) SetCelPosition(cel ID, x, y int) error {
	_, err := e.celForWrite(cel)
	if err != nil {
		return err
	}
	i, _ := e.work.cel(cel)
	e.work.Cels[i].X = x
	e.work.Cels[i].Y = y
	e.structural = true
	return nil
}
func (e *Edit) DuplicateFrame(source ID, linked bool) (ID, error) {
	if err := e.check(); err != nil {
		return "", err
	}
	i, ok := e.work.frame(source)
	if !ok {
		return "", fmt.Errorf("art: missing frame")
	}
	id, err := e.AddFrame(source, e.work.Frames[i].DurationMS)
	if err != nil {
		return "", err
	}
	old := append([]Cel(nil), e.work.Cels...)
	for _, c := range old {
		if c.Frame == source {
			if _, err = e.CopyCel(c.ID, c.Layer, id, linked); err != nil {
				return "", err
			}
		}
	}
	return id, nil
}
func (e *Edit) SetTag(tag Tag) (ID, error) {
	if err := e.check(); err != nil {
		return "", err
	}
	from, a := e.work.frame(tag.From)
	to, b := e.work.frame(tag.To)
	if !a || !b || from > to || len(tag.Name) == 0 || len(tag.Name) > 256 ||
		(tag.Direction != Forward && tag.Direction != Reverse && tag.Direction != PingPong) {
		return "", fmt.Errorf("art: invalid animation tag")
	}
	if tag.ID != "" {
		for i, t := range e.work.Tags {
			if t.ID == tag.ID {
				e.work.Tags[i] = tag
				e.structural = true
				return tag.ID, nil
			}
		}
		return "", fmt.Errorf("art: missing tag")
	}
	tag.ID = e.doc.newID()
	e.work.Tags = append(e.work.Tags, tag)
	e.structural = true
	return tag.ID, nil
}
func (e *Edit) SetSlice(v Slice) (ID, error) {
	if err := e.check(); err != nil {
		return "", err
	}
	if v.ID != "" {
		for i, s := range e.work.Slices {
			if s.ID == v.ID {
				e.work.Slices[i] = v
				e.structural = true
				return v.ID, nil
			}
		}
		return "", fmt.Errorf("art: missing slice")
	}
	v.ID = e.doc.newID()
	e.work.Slices = append(e.work.Slices, v)
	e.structural = true
	return v.ID, nil
}

func (e *Edit) DeleteSlice(id ID) error {
	if err := e.check(); err != nil {
		return err
	}
	for i, slice := range e.work.Slices {
		if slice.ID == id {
			e.work.Slices = append(e.work.Slices[:i], e.work.Slices[i+1:]...)
			e.structural = true
			return nil
		}
	}
	return fmt.Errorf("art: missing slice")
}

func (e *Edit) collect() {
	used := make(map[ID]bool)
	for _, c := range e.work.Cels {
		used[c.Image] = true
	}
	for id := range e.work.Images {
		if !used[id] {
			delete(e.work.Images, id)
		}
	}
	for id, keys := range e.mutable {
		p := e.work.Images[id]
		if p == nil {
			continue
		}
		before := e.base.Images[id]
		for key := range keys {
			t := p.Tiles[key]
			if t == nil {
				continue
			}
			if before != nil && before.Tiles[key] != nil && *t == *before.Tiles[key] {
				p.Tiles[key] = before.Tiles[key]
			} else if *t == (tile{}) {
				delete(p.Tiles, key)
			}
		}
	}
}

// Prepare seals but does not publish the transaction. Hosts route the returned
// Change through their existing history/proposal service before Apply.
func (e *Edit) Prepare() (*Change, error) {
	if err := e.check(); err != nil {
		return nil, err
	}
	e.collect()
	if err := e.work.validate(e.doc.limits); err != nil {
		return nil, err
	}
	e.doc.mu.Lock()
	defer e.doc.mu.Unlock()
	if e.doc.closed {
		return nil, ErrClosed
	}
	if e.doc.revision != e.revision || e.doc.current != e.base {
		return nil, ErrConflict
	}
	e.done = true
	if reflect.DeepEqual(e.base, e.work) {
		return nil, nil
	}
	size := retainedChangeBytes(e.base, e.work)
	damage := e.dirty
	if e.structural {
		damage = image.Rect(0, 0, e.work.Canvas.Width, e.work.Canvas.Height)
	}
	return &Change{doc: e.doc, before: e.base, after: e.work, expected: e.revision, label: e.label, bytes: size, dirty: damage}, nil
}

// Change retains immutable before/after states. Undo restores both pixels and
// topology, including link groups. Revision is monotonic even across Undo.
type Change struct {
	doc           *Document
	before, after *state
	expected      uint64
	label         string
	bytes         int64
	dirty         image.Rectangle
	status        uint8
}

func (c *Change) Label() string              { return c.label }
func (c *Change) Bytes() int64               { return c.bytes }
func (c *Change) DirtyRect() image.Rectangle { return c.dirty }
func (c *Change) Apply() error {
	d := c.doc
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	if c.status == 1 || d.current != c.before || (c.status == 0 && d.revision != c.expected) {
		return ErrConflict
	}
	d.current = c.after
	d.revision++
	c.status = 1
	return nil
}
func (c *Change) Undo() error {
	d := c.doc
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	if c.status != 1 || d.current != c.after {
		return ErrConflict
	}
	d.current = c.before
	d.revision++
	c.status = 2
	return nil
}

// Journal is the single history host for standalone/web clients. The native
// adapter must use history.History instead, not a Journal alongside it.
type Journal struct {
	undo, redo []*Change
	bytes      int64
	maxBytes   int64
	maxSteps   int
}

func NewJournal(l Limits) *Journal {
	l = limitsOrDefault(l)
	return &Journal{maxBytes: l.MaxHistoryBytes, maxSteps: l.MaxHistorySteps}
}
func (j *Journal) Exec(c *Change) error {
	if c == nil {
		return nil
	}
	if c.Bytes() > j.maxBytes {
		return fmt.Errorf("art: operation exceeds undo budget")
	}
	if err := c.Apply(); err != nil {
		return err
	}
	for _, r := range j.redo {
		j.bytes -= r.Bytes()
	}
	j.redo = nil
	j.undo = append(j.undo, c)
	j.bytes += c.Bytes()
	for len(j.undo) > j.maxSteps || j.bytes > j.maxBytes {
		j.bytes -= j.undo[0].Bytes()
		j.undo[0] = nil
		j.undo = j.undo[1:]
	}
	return nil
}
func (j *Journal) Undo() error {
	n := len(j.undo)
	if n == 0 {
		return nil
	}
	c := j.undo[n-1]
	if err := c.Undo(); err != nil {
		return err
	}
	j.undo[n-1] = nil
	j.undo = j.undo[:n-1]
	j.redo = append(j.redo, c)
	return nil
}
func (j *Journal) Redo() error {
	n := len(j.redo)
	if n == 0 {
		return nil
	}
	c := j.redo[n-1]
	if err := c.Apply(); err != nil {
		return err
	}
	j.redo[n-1] = nil
	j.redo = j.redo[:n-1]
	j.undo = append(j.undo, c)
	return nil
}
func (j *Journal) Counts() (int, int) { return len(j.undo), len(j.redo) }
func (j *Journal) Clear()             { j.undo = nil; j.redo = nil; j.bytes = 0 }
