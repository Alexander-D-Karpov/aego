package core

import (
	"aego/core/mathx"
	"aego/platform/input"
	"aego/render/icons"
	"aego/render/text"
	"aego/ui/draw"
	"aego/ui/textedit"
	"aego/ui/theme"
)

type Theme = theme.Theme

func DefaultTheme() Theme {
	return theme.Default()
}

type Cursor struct {
	X, Y  float32
	W     float32
	RowH  float32
	Pad   float32
	Space float32
}

func (c *Cursor) Next(h float32) mathx.Rect {
	if h <= 0 {
		h = c.RowH
	}
	r := mathx.Rect{X: c.X, Y: c.Y, W: c.W, H: h}
	c.Y += h + c.Space
	return r
}

type CursorShape uint8

const (
	CursorDefault CursorShape = iota
	CursorText
	CursorPointer
	CursorMove
	CursorResizeEW
	CursorResizeNS
	CursorResizeNWSE
	CursorResizeNESW
	CursorNotAllowed
	CursorCrosshair
)

type Align uint8

const (
	AlignLeft Align = iota
	AlignCenter
	AlignRight
)

type Clipboard interface {
	Text() string
	SetText(string)
}

type popup struct {
	id     ID
	at     mathx.Vec2
	h      float32
	rect   mathx.Rect
	opened uint64
	bg     int
	shadow int
	need   float32
	cur    float32
	probe  bool
}

type Mark struct {
	ids      int
	cursors  int
	clips    int
	overlays int
	popups   int
	layers   int
	fonts    int
	modal    bool
}

const (
	maxOverlays = 16
	doubleClick = 0.35
)

type Context struct {
	Theme     Theme
	Clipboard Clipboard
	Icons     *icons.Set
	Time      float64
	Flash     string
	FlashAt   float64
	FlashY    float32
	FlashSeen bool

	font  *text.Font
	fonts *text.FontSet

	mono      *text.FontSet
	monoFont  *text.Font
	fontStack []fontFrame

	list  *draw.List
	in    *input.State
	mouse mathx.Vec2
	size  mathx.Vec2
	scale float32
	frame uint64

	hot      ID
	active   ID
	captured ID
	pressed  ID
	focus    ID

	lost         ID
	lostText     string
	lostHasText  bool
	lostNext     ID
	lostNextText string
	lostNextHas  bool

	cursor  Cursor
	cursors []Cursor
	ids     []ID
	clips   []mathx.Rect

	wantsMouse bool
	wantsText  bool
	textRect   mathx.Rect
	textCursor int
	comp       string
	compCursor int
	shape      CursorShape

	modal   ID
	inModal bool

	overlays     [maxOverlays]mathx.Rect
	overlayN     int
	nextOverlays [maxOverlays]mathx.Rect
	nextN        int
	overlayDepth int
	overlayClips []int

	edit       *textedit.Buffer
	editID     ID
	editScroll float32

	lastClickID   ID
	lastClickTime float64
	doubleID      ID

	pressPos   mathx.Vec2
	prevHot    ID
	hotSince   float64
	dragValue  float64
	popups     []popup
	popupStack []int

	inFrame bool
}

func New(font *text.Font) *Context {
	return &Context{
		Theme: DefaultTheme(),
		font:  font,
		list:  draw.NewList(256),
		scale: 1,
	}
}

func (c *Context) SetFonts(set *text.FontSet) {
	c.fonts = set
	if set != nil {
		if f := set.Closest(c.Theme.FontSize, c.scale); f != nil {
			c.font = f
		}
	}
}

func (c *Context) SetFont(f *text.Font) {
	c.font = f
}

func (c *Context) Begin(in *input.State, size mathx.Vec2, scale float32) {
	if scale <= 0 {
		scale = 1
	}
	c.in = in
	c.size = size
	c.scale = scale
	c.frame++
	c.list.Reset()
	c.wantsMouse = false
	c.wantsText = false
	c.shape = CursorDefault
	c.inFrame = true
	c.mouse = mathx.Vec2{}
	if in != nil {
		c.mouse = in.MousePos()
	}
	c.hot = 0
	c.pressed = 0
	c.doubleID = 0
	c.ids = c.ids[:0]
	c.cursors = c.cursors[:0]
	c.clips = c.clips[:0]
	c.overlayClips = c.overlayClips[:0]
	c.popupStack = c.popupStack[:0]
	c.overlayDepth = 0
	c.inModal = false
	c.overlays = c.nextOverlays
	c.overlayN = c.nextN
	c.nextN = 0
	c.lost, c.lostText, c.lostHasText = c.lostNext, c.lostNextText, c.lostNextHas
	c.lostNext, c.lostNextText, c.lostNextHas = 0, "", false
	if c.fonts != nil {
		if f := c.fonts.Closest(c.Theme.FontSize, scale); f != nil {
			c.font = f
		}
	}
	c.fontStack = c.fontStack[:0]
	c.pickMono()
	c.cursor = Cursor{
		X:     c.Theme.Padding,
		Y:     c.Theme.Padding,
		W:     200,
		RowH:  c.Theme.RowHeight,
		Pad:   c.Theme.Padding,
		Space: c.Theme.Spacing,
	}
}

func (c *Context) End() *draw.List {
	c.inFrame = false
	if c.hot != c.prevHot {
		c.prevHot = c.hot
		c.hotSince = c.Time
	}
	if c.in != nil {
		if c.in.ButtonPressed(input.MouseLeft) && c.focus != 0 && c.pressed != c.focus {
			c.markLost(c.focus)
			c.focus = 0
		}
		down := c.in.ButtonDown(input.MouseLeft) || c.in.ButtonDown(input.MouseRight) || c.in.ButtonDown(input.MouseMiddle)
		if !down {
			c.captured = 0
			if !c.in.ButtonReleased(input.MouseLeft) {
				c.active = 0
			}
		}
	}
	return c.list
}

func (c *Context) Frame() uint64            { return c.frame }
func (c *Context) List() *draw.List         { return c.list }
func (c *Context) Font() *text.Font         { return c.font }
func (c *Context) Fonts() *text.FontSet     { return c.fonts }
func (c *Context) Scale() float32           { return c.scale }
func (c *Context) Size() mathx.Vec2         { return c.size }
func (c *Context) Input() *input.State      { return c.in }
func (c *Context) Hot() ID                  { return c.hot }
func (c *Context) Active() ID               { return c.active }
func (c *Context) Captured() ID             { return c.captured }
func (c *Context) Focus() ID                { return c.focus }
func (c *Context) WantsMouse() bool         { return c.wantsMouse }
func (c *Context) WantsKeyboard() bool      { return c.focus != 0 }
func (c *Context) Mouse() mathx.Vec2        { return c.mouse }
func (c *Context) Hovered(id ID) bool       { return c.hot == id }
func (c *Context) CursorShape() CursorShape { return c.shape }

func (c *Context) SetCursorShape(s CursorShape) {
	c.shape = s
}

func (c *Context) Cursor() *Cursor {
	return &c.cursor
}

func (c *Context) PushCursor(cur Cursor) {
	c.cursors = append(c.cursors, c.cursor)
	c.cursor = cur
}

func (c *Context) PopCursor() {
	if n := len(c.cursors); n > 0 {
		c.cursor = c.cursors[n-1]
		c.cursors = c.cursors[:n-1]
	}
}

func (c *Context) Region(r mathx.Rect) {
	p := c.Theme.Padding
	c.PushCursor(Cursor{
		X: r.X + p, Y: r.Y + p, W: r.W - 2*p,
		RowH: c.Theme.RowHeight, Pad: p, Space: c.Theme.Spacing,
	})
}

func (c *Context) top() ID {
	if n := len(c.ids); n > 0 {
		return c.ids[n-1]
	}
	return ID(fnvOffset)
}

func (c *Context) ID(label string) ID {
	h := uint64(c.top())
	for i := 0; i < len(label); i++ {
		h ^= uint64(label[i])
		h *= fnvPrime
	}
	return ID(h)
}

func (c *Context) IDInt(i int) ID {
	return IDSeq(c.top(), i)
}

func (c *Context) PushID(label string) {
	c.ids = append(c.ids, c.ID(label))
}

func (c *Context) PushIDInt(i int) {
	c.ids = append(c.ids, c.IDInt(i))
}

func (c *Context) PushRawID(id ID) {
	c.ids = append(c.ids, id)
}

func (c *Context) PopID() {
	if n := len(c.ids); n > 0 {
		c.ids = c.ids[:n-1]
	}
}

func intersect(a, b mathx.Rect) mathx.Rect {
	x0 := max(a.X, b.X)
	y0 := max(a.Y, b.Y)
	x1 := min(a.X+a.W, b.X+b.W)
	y1 := min(a.Y+a.H, b.Y+b.H)
	return mathx.Rect{X: x0, Y: y0, W: max(0, x1-x0), H: max(0, y1-y0)}
}

func (c *Context) PushClip(r mathx.Rect) {
	if n := len(c.clips); n > 0 {
		r = intersect(r, c.clips[n-1])
	}
	c.clips = append(c.clips, r)
	c.list.PushClip(r)
}

func (c *Context) PopClip() {
	if n := len(c.clips); n > 0 {
		c.clips = c.clips[:n-1]
		c.list.PopClip()
	}
}

func (c *Context) ClipRect() (mathx.Rect, bool) {
	if n := len(c.clips); n > 0 {
		return c.clips[n-1], true
	}
	return mathx.Rect{}, false
}

func (c *Context) Hit(r mathx.Rect) bool {
	if !r.Contains(c.mouse) {
		return false
	}
	if clip, ok := c.ClipRect(); ok && !clip.Contains(c.mouse) {
		return false
	}
	return true
}

func (c *Context) blocked() bool {
	if c.modal != 0 && !c.inModal {
		return true
	}
	if c.overlayDepth > 0 {
		return false
	}
	for i := 0; i < c.overlayN; i++ {
		if c.overlays[i].Contains(c.mouse) {
			return true
		}
	}
	return false
}

func (c *Context) MouseInOverlay() bool {
	for i := 0; i < c.overlayN; i++ {
		if c.overlays[i].Contains(c.mouse) {
			return true
		}
	}
	return false
}

func (c *Context) MouseOver(id ID, r mathx.Rect) bool {
	if c.blocked() || !c.Hit(r) {
		return false
	}
	if c.captured != 0 && c.captured != id {
		return false
	}
	if c.active == 0 || c.active == id {
		c.hot = id
	}
	c.wantsMouse = true
	return true
}

func (c *Context) Pressed(id ID, r mathx.Rect) bool {
	if !c.MouseOver(id, r) || c.in == nil || !c.in.ButtonPressed(input.MouseLeft) {
		return false
	}
	if c.active != 0 && c.active != id {
		return false
	}
	c.active = id
	c.pressed = id
	c.pressPos = c.mouse
	if c.lastClickID == id && c.Time-c.lastClickTime < doubleClick {
		c.doubleID = id
		c.lastClickID = 0
	} else {
		c.lastClickID = id
		c.lastClickTime = c.Time
	}
	return true
}

func (c *Context) DoubleClicked(id ID) bool {
	return c.doubleID == id
}

func (c *Context) Clicked(id ID, r mathx.Rect) bool {
	c.Pressed(id, r)
	if c.in == nil {
		return false
	}
	if c.active == id && c.in.ButtonReleased(input.MouseLeft) {
		c.active = 0
		return c.Hit(r) && !c.blocked()
	}
	return false
}

func (c *Context) RightClicked(id ID, r mathx.Rect) bool {
	return c.MouseOver(id, r) && c.in != nil && c.in.ButtonPressed(input.MouseRight)
}

func (c *Context) Drag(id ID, r mathx.Rect) (mathx.Vec2, bool) {
	c.Pressed(id, r)
	if c.in == nil || c.active != id {
		return mathx.Vec2{}, false
	}
	c.wantsMouse = true
	if c.in.ButtonReleased(input.MouseLeft) && !c.in.ButtonDown(input.MouseLeft) {
		c.active = 0
	}
	return c.in.MouseDelta(), true
}

func (c *Context) CaptureMouse(id ID) {
	c.captured = id
	c.active = id
}

func (c *Context) ReleaseMouse() {
	c.captured = 0
	c.active = 0
}

func (c *Context) MouseDown(b input.Button) bool {
	return c.in != nil && c.in.ButtonDown(b)
}

func (c *Context) MousePressed(b input.Button) bool {
	return c.in != nil && c.in.ButtonPressed(b)
}

func (c *Context) MouseReleased(b input.Button) bool {
	return c.in != nil && c.in.ButtonReleased(b)
}

func (c *Context) MouseDelta() mathx.Vec2 {
	if c.in == nil {
		return mathx.Vec2{}
	}
	return c.in.MouseDelta()
}

type Scroll struct {
	Lines  mathx.Vec2
	Pixels mathx.Vec2
}

const (
	PrecisePixels = 10
	PreciseZoom   = 0.25
)

func (c *Context) Scroll() Scroll {
	if c.in == nil {
		return Scroll{}
	}
	return Scroll{Lines: c.in.WheelLines(), Pixels: c.in.WheelPrecise().Scale(PrecisePixels)}
}

func (c *Context) ScrollY(step float32) float32 {
	s := c.Scroll()
	return s.Lines.Y*step + s.Pixels.Y
}

func (c *Context) ScrollX(step float32) float32 {
	s := c.Scroll()
	return s.Lines.X*step + s.Pixels.X
}

func (c *Context) ZoomY() float32 {
	if c.in == nil {
		return 0
	}
	z := c.in.WheelLines().Y + c.in.WheelPrecise().Y*PreciseZoom
	if c.in.WheelFlipped() {
		z = -z
	}
	return z
}

func (c *Context) SetShapes(s *draw.Shapes) {
	c.list.SetShapes(s)
}

func (c *Context) RoundRect(r mathx.Rect, radius float32, col mathx.Color) {
	c.list.RoundRect(r, radius, col, draw.CornersAll)
}

func (c *Context) Box(r mathx.Rect, radius float32, fill, border mathx.Color) {
	if radius <= 0 || c.list.Shapes() == nil {
		c.list.Rect(r, fill)
		c.list.RectOutline(r, border, 1)
		return
	}
	c.list.Box(r, radius, fill, border)
}

func (c *Context) Shadow(r mathx.Rect, radius float32) {
	th := &c.Theme
	if th.Shadow.A > 0 {
		c.list.Shadow(r, radius, th.ShadowBlur, th.ShadowY, th.Shadow)
	}
}

func (c *Context) KeyPressed(k input.Scancode) bool {
	return c.in != nil && c.in.KeyPressed(k)
}

func (c *Context) KeyRepeated(k input.Scancode) bool {
	return c.in != nil && c.in.KeyRepeated(k)
}

func (c *Context) KeyRepeat(k input.Scancode) bool {
	return c.KeyRepeated(k)
}

func (c *Context) Mods() input.Mods {
	if c.in == nil {
		return 0
	}
	return c.in.Mods()
}

func (c *Context) Primary() bool {
	return c.Mods().Primary()
}

func (c *Context) WordMod() input.Mods {
	return input.WordMod()
}

func (c *Context) TextInput() []byte {
	if c.in == nil {
		return nil
	}
	return c.in.Text()
}

func (c *Context) WantTextInput(r mathx.Rect, cursor int) {
	c.wantsText = true
	c.textRect = r
	c.textCursor = cursor
}

func (c *Context) WantsTextInput() (bool, mathx.Rect, int) {
	return c.wantsText, c.textRect, c.textCursor
}

func (c *Context) SetComposition(s string, cursor int) {
	c.comp = s
	c.compCursor = cursor
}

func (c *Context) Composition() (string, int) {
	return c.comp, c.compCursor
}

func (c *Context) markLost(id ID) {
	c.lostNext = id
	c.lostNextText, c.lostNextHas = "", false
	if c.editID == id && c.edit != nil {
		c.lostNextText = c.edit.String()
		c.lostNextHas = true
		c.editID = 0
	}
	c.comp, c.compCursor = "", 0
}

func (c *Context) SetFocus(id ID) {
	if c.focus != 0 && c.focus != id {
		c.markLost(c.focus)
	}
	c.focus = id
}

func (c *Context) CancelFocus() {
	c.focus = 0
	c.editID = 0
	c.comp, c.compCursor = "", 0
}

func (c *Context) LostFocus(id ID) bool {
	return id != 0 && c.lost == id
}

func (c *Context) LostText(id ID) (string, bool) {
	if id == 0 || c.lost != id || !c.lostHasText {
		return "", false
	}
	return c.lostText, true
}

func (c *Context) EditBuffer(id ID, initial string) *textedit.Buffer {
	if c.edit == nil {
		c.edit = textedit.New("")
	}
	if c.editID != id {
		c.editID = id
		c.edit.Set(initial)
		c.editScroll = 0
	}
	return c.edit
}

func (c *Context) EditingID() ID {
	return c.editID
}

func (c *Context) EndEdit() {
	c.editID = 0
}

func (c *Context) EditScroll() *float32 {
	return &c.editScroll
}

func (c *Context) OpenModal(id ID) {
	c.modal = id
}

func (c *Context) CloseModal() {
	c.modal = 0
}

func (c *Context) ModalOpen(id ID) bool {
	return id != 0 && c.modal == id
}

func (c *Context) ModalActive() bool {
	return c.modal != 0
}

func (c *Context) BeginModal(id ID, r mathx.Rect) bool {
	if c.modal != id {
		return false
	}
	c.inModal = true
	full := mathx.Rect{W: c.size.X, H: c.size.Y}
	c.BeginOverlay(full, draw.LayerModal)
	c.list.Rect(full, c.Theme.Overlay)
	c.Shadow(r, c.Theme.Radius.Popup)
	c.Box(r, c.Theme.Radius.Popup, c.Theme.Panel, c.Theme.Border)
	c.PushClip(r)
	c.Region(r)
	return true
}

func (c *Context) EndModal() {
	c.PopCursor()
	c.PopClip()
	c.EndOverlay()
	c.inModal = false
}

func (c *Context) BeginOverlay(r mathx.Rect, layer int) {
	if c.nextN < maxOverlays {
		c.nextOverlays[c.nextN] = r
		c.nextN++
	}
	c.overlayDepth++
	c.overlayClips = append(c.overlayClips, len(c.clips))
	c.clips = append(c.clips, r)
	c.list.PushLayer(layer)
	c.list.PushClipAbs(r)
}

func (c *Context) EndOverlay() {
	n := len(c.overlayClips)
	if n == 0 {
		return
	}
	base := c.overlayClips[n-1]
	c.overlayClips = c.overlayClips[:n-1]
	for len(c.clips) > base+1 {
		c.PopClip()
	}
	c.clips = c.clips[:base]
	c.list.PopClip()
	c.list.PopLayer()
	c.overlayDepth--
}

func (c *Context) InOverlay() bool {
	return c.overlayDepth > 0
}

func (c *Context) PressPos() mathx.Vec2 {
	return c.pressPos
}

func (c *Context) DragValue() *float64 {
	return &c.dragValue
}

func (c *Context) HoverDuration(id ID) float64 {
	if id == 0 || c.hot != id || c.prevHot != id {
		return 0
	}
	return c.Time - c.hotSince
}

func (c *Context) popupIndex(id ID) int {
	for i := range c.popups {
		if c.popups[i].id == id {
			return i
		}
	}
	return -1
}

func (c *Context) OpenPopup(id ID, at mathx.Vec2) {
	if i := c.popupIndex(id); i >= 0 {
		c.popups = c.popups[:i]
	}
	c.popups = append(c.popups, popup{id: id, at: at, opened: c.frame})
}

func (c *Context) PopupOpen(id ID) bool {
	return c.popupIndex(id) >= 0
}

func (c *Context) AnyPopup() bool {
	return len(c.popups) > 0
}

func (c *Context) ClosePopup(id ID) {
	if i := c.popupIndex(id); i >= 0 {
		c.popups = c.popups[:i]
	}
}

func (c *Context) CloseAllPopups() {
	c.popups = c.popups[:0]
}

func (c *Context) CloseChildren() {
	n := len(c.popupStack)
	if n == 0 {
		return
	}
	if i := c.popupStack[n-1]; i+1 < len(c.popups) {
		c.popups = c.popups[:i+1]
	}
}

func (c *Context) insidePopups(from int) bool {
	for j := from; j < len(c.popups); j++ {
		if c.popups[j].rect.Contains(c.mouse) {
			return true
		}
	}
	return false
}

func (c *Context) BeginPopup(id ID, w float32) bool {
	i := c.popupIndex(id)
	if i < 0 {
		return false
	}
	p := &c.popups[i]
	w = max(w, p.need)
	p.cur = 0
	if c.frame != p.opened && c.in != nil {
		pressed := c.in.ButtonPressed(input.MouseLeft) || c.in.ButtonPressed(input.MouseRight)
		if pressed && !c.insidePopups(i) {
			c.popups = c.popups[:i]
			return false
		}
		if i == len(c.popups)-1 && c.in.KeyPressed(input.KeyEscape) {
			c.popups = c.popups[:i]
			return false
		}
	}
	h := p.h
	p.probe = h <= 0
	if p.probe {
		h = min(float32(400), c.size.Y)
	}
	r := mathx.Rect{X: p.at.X, Y: p.at.Y, W: w, H: h}
	if r.X+r.W > c.size.X {
		r.X = max(0, c.size.X-r.W)
	}
	if r.Y+r.H > c.size.Y {
		r.Y = max(0, c.size.Y-r.H)
	}
	r.X, r.Y, r.W = float32(int(r.X+0.5)), float32(int(r.Y+0.5)), float32(int(r.W+0.5))
	p.rect = r
	layer := draw.LayerPopup
	if c.overlayDepth > 0 {
		layer = draw.LayerTop
	}
	c.list.PushLayer(layer)
	p.shadow = c.list.ReserveShadow()
	c.list.PopLayer()
	c.BeginOverlay(r, layer)
	p.bg = c.list.ReserveBox()
	c.Region(r)
	if p.probe {
		c.PushClip(mathx.Rect{X: r.X, Y: r.Y})
	}
	c.popupStack = append(c.popupStack, i)
	return true
}

func (c *Context) EndPopup() {
	n := len(c.popupStack)
	if n == 0 {
		return
	}
	i := c.popupStack[n-1]
	c.popupStack = c.popupStack[:n-1]
	bottom := c.cursor.Y
	c.PopCursor()
	if i < len(c.popups) {
		th := &c.Theme
		p := &c.popups[i]
		p.need = p.cur + 2*th.Padding
		p.h = bottom - p.rect.Y + th.Padding - th.Spacing
		r := p.rect
		r.H = min(p.h, r.H)
		if p.probe {
			c.PopClip()
			r = mathx.Rect{X: r.X, Y: r.Y}
		}
		c.list.PatchShadow(p.shadow, r, th.Radius.Popup, th.ShadowBlur, th.ShadowY, th.Shadow)
		c.list.PatchBox(p.bg, r, th.Radius.Popup, th.PanelAlt, th.Border)
	}
	c.EndOverlay()
}

func (c *Context) FitPopup(contentW float32) {
	n := len(c.popupStack)
	if n == 0 {
		return
	}
	if i := c.popupStack[n-1]; i < len(c.popups) {
		if p := &c.popups[i]; contentW > p.cur {
			p.cur = contentW
		}
	}
}

type fontFrame struct {
	ui   *text.Font
	list *text.Font
}

func (c *Context) SetMonoFonts(set *text.FontSet) {
	c.mono = set
	c.pickMono()
}

func (c *Context) pickMono() {
	c.monoFont = nil
	if c.mono != nil {
		c.monoFont = c.mono.Closest(c.Theme.FontSize, c.scale)
	}
}

func (c *Context) MonoFont() *text.Font {
	if c.monoFont != nil {
		return c.monoFont
	}
	return c.font
}

func (c *Context) FontAt(px float32) *text.Font {
	if c.fonts != nil {
		if f := c.fonts.Closest(px, c.scale); f != nil {
			return f
		}
	}
	return c.font
}

func (c *Context) MonoFontAt(px float32) *text.Font {
	if c.mono != nil {
		if f := c.mono.Closest(px, c.scale); f != nil {
			return f
		}
	}
	return c.MonoFont()
}

func (c *Context) PushFont(f *text.Font) {
	c.fontStack = append(c.fontStack, fontFrame{ui: c.font, list: c.list.Font()})
	if f != nil {
		c.font = f
		c.list.SetFont(f)
	}
}

func (c *Context) PopFont() {
	n := len(c.fontStack)
	if n == 0 {
		return
	}
	fr := c.fontStack[n-1]
	c.fontStack = c.fontStack[:n-1]
	c.font = fr.ui
	c.list.SetFont(fr.list)
}

func (c *Context) Mark() Mark {
	return Mark{
		ids:      len(c.ids),
		cursors:  len(c.cursors),
		clips:    len(c.clips),
		overlays: len(c.overlayClips),
		popups:   len(c.popupStack),
		layers:   c.list.LayerDepth(),
		fonts:    len(c.fontStack),
		modal:    c.inModal,
	}
}

func (c *Context) Restore(m Mark) {
	if len(c.popupStack) > m.popups {
		c.popupStack = c.popupStack[:m.popups]
	}
	for len(c.overlayClips) > m.overlays {
		c.EndOverlay()
	}
	for len(c.clips) > m.clips {
		c.PopClip()
	}
	for len(c.fontStack) > m.fonts {
		c.PopFont()
	}
	if len(c.ids) > m.ids {
		c.ids = c.ids[:m.ids]
	}
	if len(c.cursors) > m.cursors {
		c.cursor = c.cursors[m.cursors]
		c.cursors = c.cursors[:m.cursors]
	}
	for c.list.LayerDepth() > m.layers {
		c.list.PopLayer()
	}
	c.inModal = m.modal
}

func (c *Context) Rect(r mathx.Rect, col mathx.Color) {
	c.list.Rect(r, col)
}

func (c *Context) Outline(r mathx.Rect, col mathx.Color) {
	c.list.RectOutline(r, col, 1)
}

func (c *Context) Text(s string, pos mathx.Vec2, col mathx.Color) {
	c.list.Text(s, pos, col)
}

func (c *Context) TextIn(r mathx.Rect, s string, col mathx.Color, align Align) {
	x := r.X + c.Theme.Padding
	switch align {
	case AlignCenter:
		x = r.X + (r.W-c.TextSize(s).X)/2
	case AlignRight:
		x = r.X + r.W - c.Theme.Padding - c.TextSize(s).X
	}
	c.list.Text(s, mathx.Vec2{X: x, Y: c.TextY(r)}, col)
}

func (c *Context) LineHeight() float32 {
	if c.font == nil {
		return 0
	}
	return c.font.LineHeight
}

func (c *Context) TextY(r mathx.Rect) float32 {
	return r.Y + (r.H-c.LineHeight())/2
}

func (c *Context) TextSize(s string) mathx.Vec2 {
	if c.font == nil {
		return mathx.Vec2{}
	}
	return c.font.Measure(s)
}

func (c *Context) WidgetColor(id ID) mathx.Color {
	switch {
	case c.active == id:
		return c.Theme.WidgetDown
	case c.hot == id:
		return c.Theme.WidgetHot
	}
	return c.Theme.Widget
}
