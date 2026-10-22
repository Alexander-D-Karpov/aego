package assets

import (
	"path"
	"slices"
	"strings"
	"unicode"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/mathx"
	"aego/editor/core/code/highlight"
	"aego/ui/core"
	"aego/ui/draw"
	"aego/ui/widgets"
)

var tileSizes = [...]float32{48, 64, 96}

const waveCols = 48

var tileLabels = [...]string{"S", "M", "L"}

func (p *Panel) tileSize() float32 {
	if p.tile <= 0 {
		return tileSizes[1]
	}
	return p.tile
}

func (p *Panel) SetTileSize(px float32) {
	p.tile = px
}

func (p *Panel) tileW() float32 {
	return p.tileSize() + 32
}

func (p *Panel) sizeButtons(r mathx.Rect) {
	ui := p.f.UI
	w := r.W / float32(len(tileSizes))
	for i, s := range tileSizes {
		on := p.tileSize() == s
		if widgets.ToggleButton(ui, core.IDSeq(ui.ID("tilesize"), i), mathx.Rect{X: r.X + float32(i)*w, Y: r.Y, W: w - 2, H: r.H}, tileLabels[i], &on) {
			p.tile = s
			p.gridScroll = 0
		}
	}
}

func kindIcon(k asset.ArtifactKind) string {
	switch k {
	case asset.ArtScene:
		return "file.scene"
	case asset.ArtPrefab:
		return "file.prefab"
	case asset.ArtScript:
		return "file.script"
	case asset.ArtTexture, asset.ArtSpriteRegion, asset.ArtAtlasPage:
		return "file.texture"
	case asset.ArtData:
		return "file.data"
	case asset.ArtTable:
		return "file.table"
	case asset.ArtMesh:
		return "file.mesh"
	case asset.ArtAudio:
		return "cat.audio"
	}
	return "file.unknown"
}

func (p *Panel) kindColor(k asset.ArtifactKind) mathx.Color {
	th := &p.f.UI.Theme
	c := th.TextDim
	switch k {
	case asset.ArtScene:
		c = th.Accent
	case asset.ArtPrefab:
		c = th.AxisY
	case asset.ArtScript:
		c = th.AxisZ
	case asset.ArtData, asset.ArtTable:
		c = th.Warn
	case asset.ArtMesh:
		c = th.AxisX
	case asset.ArtAudio:
		c = th.Ok
	}
	return c
}

func (p *Panel) checker(r mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	ui.Rect(r, th.PanelAlt)
	const cell = float32(8)
	dark := th.Widget
	ui.PushClip(r)
	for y, row := r.Y, 0; y < r.Y+r.H; y, row = y+cell, row+1 {
		for x, col := r.X, 0; x < r.X+r.W; x, col = x+cell, col+1 {
			if (row+col)%2 == 0 {
				ui.Rect(mathx.Rect{X: x, Y: y, W: cell, H: cell}, dark)
			}
		}
	}
	ui.PopClip()
}

func (p *Panel) schematic(a *pipeline.AssetInfo, img mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	tint := p.kindColor(a.Kind)
	rad := th.Radius.Button
	ui.RoundRect(img, rad, th.Border)
	card := mathx.Rect{X: img.X + 1, Y: img.Y + 1, W: img.W - 2, H: img.H - 2}
	ui.RoundRect(card, rad, th.CardBg)
	ui.List().RoundRect(mathx.Rect{X: card.X, Y: card.Y, W: card.W, H: 3}, rad, tint, draw.CornersTop)
	if a.Kind == asset.ArtScript {
		if lines := p.d.Thumbs.Lines(*a); len(lines) > 0 {
			p.code(lines, mathx.Rect{X: card.X + 4, Y: card.Y + 6, W: card.W - 8, H: card.H - 9})
			return
		}
	}
	sub := ""
	if a.Kind == asset.ArtData || a.Kind == asset.ArtTable {
		if p.d.DataType != nil {
			if t, ok := p.d.DataType(a.ID); ok {
				sub = t[max(0, lastDot(t)+1):]
			}
		}
	}
	showSub := sub != "" && card.H >= 44
	icon := min(card.W*0.38, 32)
	cy := card.Y + 3 + (card.H-3-icon)/2
	var small float32
	if showSub {
		ui.PushFont(ui.FontAt(11))
		small = ui.LineHeight()
		ui.PopFont()
		cy -= small / 2
	}
	widgets.IconAt(ui, mathx.Rect{X: card.X + (card.W-icon)/2, Y: cy, W: icon, H: icon}, kindIcon(a.Kind), tint)
	if showSub {
		ui.PushFont(ui.FontAt(11))
		sr := mathx.Rect{X: card.X + 3, Y: cy + icon + 2, W: card.W - 6, H: small}
		ui.TextIn(sr, widgets.Elide(ui, sub, sr.W), th.TextDim, core.AlignCenter)
		ui.PopFont()
		return
	}
	p.badge(a, card, tint)
}

func lastDot(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}

func (p *Panel) stack(dir string) []int {
	if p.stackRev != p.rev || p.stacks == nil {
		p.stacks = map[string][]int{}
		p.stackRev = p.rev
	}
	if s, ok := p.stacks[dir]; ok {
		return s
	}
	var out []int
	for i := range p.items {
		it := &p.items[i]
		if path.Dir(it.Path) != dir || it.Kind == asset.ArtScript || it.Kind == asset.ArtData || it.Kind == asset.ArtTable {
			continue
		}
		out = append(out, i)
		if len(out) == 4 {
			break
		}
	}
	p.stacks[dir] = slices.Clip(out)
	return p.stacks[dir]
}

// thumb draws an asset's picture into r: an audio waveform, a cached thumbnail, or false when neither is ready.
func (p *Panel) thumb(a *pipeline.AssetInfo, r mathx.Rect) bool {
	if a.Kind == asset.ArtAudio {
		return p.wave(a, r)
	}
	reg, ok := p.d.Thumbs.Get(*a)
	if !ok {
		return false
	}
	p.f.UI.List().Texture(reg, fitRect(r, reg.W, reg.H), mathx.Color{R: 1, G: 1, B: 1, A: 1})
	return true
}

func (p *Panel) wave(a *pipeline.AssetInfo, r mathx.Rect) bool {
	peaks, _ := p.d.Thumbs.Peaks(a.ID, waveCols)
	if len(peaks) == 0 {
		return false
	}
	ui := p.f.UI
	col := p.kindColor(a.Kind)
	w := r.W / float32(len(peaks))
	mid := r.Y + r.H/2
	for i, x := range peaks {
		h := max(min(x, 1)*r.H/2, 0.5)
		ui.Rect(mathx.Rect{X: r.X + float32(i)*w, Y: mid - h, W: max(w, 1), H: 2 * h}, col)
	}
	return true
}

func (p *Panel) audioCard(a *pipeline.AssetInfo, img mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	tint := p.kindColor(a.Kind)
	rad := th.Radius.Button
	ui.RoundRect(img, rad, th.Border)
	card := mathx.Rect{X: img.X + 1, Y: img.Y + 1, W: img.W - 2, H: img.H - 2}
	ui.RoundRect(card, rad, th.CardBg)
	ui.List().RoundRect(mathx.Rect{X: card.X, Y: card.Y, W: card.W, H: 3}, rad, tint, draw.CornersTop)
	band := mathx.Rect{X: card.X + 4, Y: card.Y + card.H*0.22, W: card.W - 8, H: card.H * 0.46}
	if !p.wave(a, band) {
		icon := min(card.W*0.38, 32)
		widgets.IconAt(ui, mathx.Rect{X: card.X + (card.W-icon)/2, Y: card.Y + 3 + (card.H-3-icon)/2, W: icon, H: icon}, kindIcon(a.Kind), tint)
	}
	p.badge(a, card, tint)
}

// folderStack draws up to three previews as sheets sticking out of the folder icon.
func (p *Panel) folderStack(dir string, img mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	icon := img.W * 0.82
	at := mathx.Rect{X: img.X + (img.W-icon)/2, Y: img.Y + (img.H-icon)/2 + icon*0.06, W: icon, H: icon}
	list := p.stack(dir)
	n := min(len(list), 3)
	if img.W < 40 {
		n = 0
	}
	cs := icon * 0.5
	step := cs * 0.62
	x0 := at.X + (at.W-cs-float32(n-1)*step)/2
	for k := 0; k < n; k++ {
		it := &p.items[list[k]]
		lift := float32(k%2) * icon * 0.06
		cr := mathx.Rect{X: x0 + float32(k)*step, Y: at.Y + icon*0.02 - lift, W: cs, H: cs}
		ui.Rect(cr, th.CardBg)
		in := mathx.Rect{X: cr.X + 2, Y: cr.Y + 2, W: cr.W - 4, H: cr.H * 0.42}
		if !p.thumb(it, in) {
			ic := cs * 0.4
			widgets.IconAt(ui, mathx.Rect{X: cr.X + (cs-ic)/2, Y: cr.Y + 2, W: ic, H: ic}, kindIcon(it.Kind), p.kindColor(it.Kind))
		}
		ui.Outline(cr, th.Border)
	}
	widgets.IconAt(ui, at, "file.folder", th.Accent)
}

func (p *Panel) code(lines []string, r mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	pal := highlight.Palette(&th.Code)
	ui.PushClip(r)
	defer ui.PopClip()
	y := r.Y
	var st highlight.State
	if name, kind, ok := declName(lines); ok && r.W >= 80 {
		ui.PushFont(ui.MonoFontAt(11))
		ui.Text(widgets.Elide(ui, name, r.W), mathx.Vec2{X: r.X, Y: y}, pal[kind])
		y += ui.LineHeight() + 3
		ui.PopFont()
		p.spans, st = highlight.Line([]byte(lines[0]), st, p.spans)
		lines = lines[1:]
	}
	lh := max(float32(2.5), r.W/24)
	cw := lh * 0.5
	bar := max(float32(1.5), lh*0.55)
	for _, l := range lines {
		if y+bar > r.Y+r.H {
			break
		}
		p.spans, st = highlight.Line([]byte(l), st, p.spans)
		for _, sp := range p.spans {
			seg := l[sp.Start:sp.End]
			col := pal[sp.Kind]
			col.A = 0.8
			start := -1
			for i := 0; i <= len(seg); i++ {
				if i < len(seg) && seg[i] != ' ' {
					if start < 0 {
						start = i
					}
					continue
				}
				if start >= 0 {
					x := r.X + float32(sp.Start+start)*cw
					if x < r.X+r.W {
						ui.Rect(mathx.Rect{X: x, Y: y, W: min(float32(i-start)*cw, r.X+r.W-x), H: bar}, col)
					}
					start = -1
				}
			}
		}
		y += lh
	}
}

func declName(lines []string) (string, highlight.Kind, bool) {
	if len(lines) == 0 {
		return "", 0, false
	}
	kw, rest, _ := strings.Cut(strings.TrimSpace(lines[0]), " ")
	kind := highlight.Kind(0)
	switch kw {
	case "func":
		kind = highlight.Function
		if strings.HasPrefix(rest, "(") {
			if _, after, ok := strings.Cut(rest, ")"); ok {
				rest, kind = strings.TrimSpace(after), highlight.Method
			}
		}
	case "const":
		kind = highlight.Constant
	case "var":
		kind = highlight.Variable
	case "type":
		kind = highlight.Type
	default:
		return "", 0, false
	}
	end := strings.IndexFunc(rest, func(r rune) bool {
		return r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if end >= 0 {
		rest = rest[:end]
	}
	return rest, kind, rest != ""
}

func (p *Panel) badge(a *pipeline.AssetInfo, card mathx.Rect, tint mathx.Color) {
	if card.W < 60 {
		return
	}
	ui := p.f.UI
	ext := strings.TrimPrefix(path.Ext(a.Path), ".")
	if len(ext) > 4 && strings.HasPrefix(ext, "ae") {
		ext = ext[2:]
	}
	ext = strings.ToUpper(ext)
	if ext == "" {
		return
	}
	ui.PushFont(ui.MonoFontAt(10))
	sz := ui.TextSize(ext)
	br := mathx.Rect{X: card.X + card.W - sz.X - 9, Y: card.Y + card.H - sz.Y - 5, W: sz.X + 6, H: sz.Y + 2}
	bg := tint
	bg.A = 0.85
	ui.RoundRect(br, 3, bg)
	ui.Text(ext, mathx.Vec2{X: br.X + 3, Y: br.Y + 1}, ui.Theme.CardBg)
	ui.PopFont()
}
