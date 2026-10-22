package references

import (
	"aego/asset"
	"path"
	"strconv"
	"strings"
	"time"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/refs"
	"aego/editor/core/services"
	"aego/ui/core"
	"aego/ui/widgets"
)

type Missing struct {
	d       Deps
	assets  []refs.Missing
	nodes   []refs.MissingNode
	rev     uint64
	at      time.Time
	scroll  float32
	pick    string
	picking int
}

func NewMissing(d Deps) *Missing {
	return &Missing{d: d, picking: -1}
}

func (m *Missing) ID() string {
	return MissingID
}

func (m *Missing) Title() string {
	return "Missing References"
}

func (m *Missing) Icon() string {
	return "status.warning"
}

func (m *Missing) Refresh() {
	m.assets = m.d.Refs.MissingAssets()
	m.nodes = m.d.Refs.MissingNodes()
	m.rev = m.d.Pipeline.Revision()
	m.at = time.Now()
}

func (m *Missing) Count() int {
	return len(m.assets) + len(m.nodes)
}

func (m *Missing) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	if m.at.IsZero() || (m.rev != m.d.Pipeline.Revision() && time.Since(m.at) > time.Second) {
		m.Refresh()
	}
	head := ui.Cursor().Next(0)
	rw := ui.TextSize("Rescan").X + 16
	ui.Text(strconv.Itoa(len(m.assets))+" missing asset(s), "+strconv.Itoa(len(m.nodes))+" missing node reference(s)", mathx.Vec2{X: head.X, Y: ui.TextY(head)}, th.TextDim)
	if widgets.ButtonRect(ui, ui.ID("rescan"), mathx.Rect{X: head.X + head.W - rw, Y: head.Y, W: rw, H: head.H}, "Rescan") {
		m.Refresh()
	}
	if m.Count() == 0 {
		widgets.LabelDim(ui, "Every reference resolves")
		return
	}
	top := ui.Cursor().Y
	list := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top)}
	rowH := th.RowHeight + 4
	total := len(m.assets) + len(m.nodes)
	widgets.VirtualList(ui, ui.ID("rows"), list, total, rowH, &m.scroll, func(i int, rr mathx.Rect) {
		if i < len(m.assets) {
			m.assetRow(ui, i, rr)
		} else {
			m.nodeRow(ui, i-len(m.assets), rr)
		}
	})
}

func (m *Missing) buttons(ui *core.Context, rr mathx.Rect, id core.ID, labels ...string) int {
	x := rr.X + rr.W
	hit := -1
	for i := len(labels) - 1; i >= 0; i-- {
		w := ui.TextSize(labels[i]).X + 14
		x -= w + 4
		if widgets.ButtonRect(ui, core.IDSeq(id, i+1), mathx.Rect{X: x, Y: rr.Y + 2, W: w, H: rr.H - 4}, labels[i]) {
			hit = i
		}
	}
	return hit
}

func (m *Missing) assetRow(ui *core.Context, i int, rr mathx.Rect) {
	th := &ui.Theme
	it := m.assets[i]
	id := core.IDSeq(ui.ID("asset"), i)
	widgets.IconAt(ui, mathx.Rect{X: rr.X + 2, Y: rr.Y + (rr.H-16)/2, W: 16, H: 16}, "status.warning", th.Warn)
	text := it.Label() + "  ·  " + strconv.Itoa(len(it.Refs)) + " referrer(s)"
	labels := []string{"Show", "Replace…", "Clear"}
	if !it.Suggest.IsZero() {
		labels = append([]string{"Use " + path.Base(it.Path)}, labels...)
	}
	bw := float32(0)
	for _, l := range labels {
		bw += ui.TextSize(l).X + 18
	}
	ui.PushClip(mathx.Rect{X: rr.X, Y: rr.Y, W: max(0, rr.W-bw), H: rr.H})
	ui.Text(text, mathx.Vec2{X: rr.X + 22, Y: ui.TextY(rr)}, th.Text)
	ui.PopClip()
	tip := "Referenced by:"
	for k, ref := range it.Refs {
		if k == 8 {
			tip += "\n…"
			break
		}
		tip += "\n" + ref.Path
	}
	ui.MouseOver(id, rr)
	widgets.Tooltip(ui, id, tip)
	hit := m.buttons(ui, rr, id, labels...)
	if hit < 0 {
		m.picker(ui, i, rr)
		return
	}
	if !it.Suggest.IsZero() {
		if hit == 0 {
			runReplace(m.d, it.ID, it.Suggest, "Resolve "+path.Base(it.Label()), m.Refresh)
			return
		}
		hit--
	}
	switch hit {
	case 0:
		if m.d.Show != nil {
			m.d.Show(it.ID)
		}
	case 1:
		m.picking, m.pick = i, ""
		ui.CloseAllPopups()
		ui.OpenPopup(core.IDSeq(ui.ID("pick"), i), mathx.Vec2{X: rr.X + rr.W/2, Y: rr.Y + rr.H})
	case 2:
		runReplace(m.d, it.ID, ids.AssetID{}, "Clear "+path.Base(it.Label()), m.Refresh)
	}
	m.picker(ui, i, rr)
}

func (m *Missing) picker(ui *core.Context, i int, rr mathx.Rect) {
	if m.picking != i {
		return
	}
	it := m.assets[i]
	if to, ok := pickAsset(ui, core.IDSeq(ui.ID("pick"), i), m.d.Pipeline, it.Grave.Kind, &m.pick, ids.AssetID{}); ok {
		m.picking = -1
		runReplace(m.d, it.ID, to, "Resolve "+path.Base(it.Label()), m.Refresh)
	}
}

func (m *Missing) nodeRow(ui *core.Context, i int, rr mathx.Rect) {
	th := &ui.Theme
	it := m.nodes[i]
	id := core.IDSeq(ui.ID("node"), i)
	widgets.IconAt(ui, mathx.Rect{X: rr.X + 2, Y: rr.Y + (rr.H-16)/2, W: 16, H: 16}, "cat.node", th.Warn)
	node := strings.TrimPrefix(it.Path, "/")
	text := it.Scene + "  ·  " + path.Base(node) + " · " + it.Owner + "." + it.Field + " → missing node"
	widgets.Tooltip(ui, id, node)
	labels := []string{"Select", "Clear"}
	ui.PushClip(mathx.Rect{X: rr.X, Y: rr.Y, W: max(0, rr.W-130), H: rr.H})
	ui.Text(text, mathx.Vec2{X: rr.X + 22, Y: ui.TextY(rr)}, th.Text)
	ui.PopClip()
	switch m.buttons(ui, rr, id, labels...) {
	case 0:
		openPlace(m.d, refs.Ref{Path: it.Scene, Kind: sceneKind(it.Scene)}, refs.Place{Node: it.Node})
	case 1:
		if err := m.d.Refs.ClearNode(it); err != nil {
			m.d.Notices.Notify(context.LevelError, err.Error())
		}
		m.Refresh()
	}
}

func sceneKind(rel string) asset.ArtifactKind {
	if path.Ext(rel) == ".aeprefab" {
		return asset.ArtPrefab
	}
	return asset.ArtScene
}
