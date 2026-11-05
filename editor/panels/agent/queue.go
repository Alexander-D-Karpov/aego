package agent

import (
	"slices"
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/ui/core"
	"aego/ui/widgets"
)

const maxQueuedMessages = 8

type queuedMessage struct {
	text, full string
	images     [][]byte
}

func (p *Panel) enqueue(msg queuedMessage) bool {
	if len(p.queue) >= maxQueuedMessages {
		p.notice(i18n.Tr("editor.agent.queue_full", "The message queue is full."))
		return false
	}
	if len(msg.images) > 0 {
		if _, ok := p.d.Host.(interface{ SendImages(string, [][]byte) error }); !ok {
			p.notice(i18n.Tr("editor.agent.image_unsupported", "This agent does not support image input."))
			return false
		}
	}
	p.queue = append(p.queue, msg)
	return true
}

func (p *Panel) removeQueued(i int) {
	p.queue = slices.Delete(p.queue, i, i+1)
	if len(p.queue) == 0 {
		p.queuePaused = false
	}
}

func (p *Panel) queueHeight(ui *core.Context) float32 {
	if len(p.queue) == 0 {
		return 0
	}
	rows := 1 + min(len(p.queue), 3)
	if len(p.queue) > 3 {
		rows++
	}
	return float32(rows) * (ui.Theme.RowHeight + ui.Theme.Spacing)
}

func (p *Panel) drawQueue(ui *core.Context, r mathx.Rect) {
	if len(p.queue) == 0 {
		return
	}
	th := &ui.Theme
	row := mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: th.RowHeight}
	clearLabel := i18n.Tr("editor.agent.queue_clear", "Clear")
	cw := ui.TextSize(clearLabel).X + 20
	if widgets.ButtonRect(ui, ui.ID("queue-clear"), mathx.Rect{X: row.X + row.W - cw, Y: row.Y, W: cw, H: row.H}, clearLabel) {
		clear(p.queue)
		p.queue = nil
		p.queuePaused = false
		return
	}
	right := row.X + row.W - cw - 6
	if p.queuePaused {
		label := i18n.Tr("editor.agent.queue_resume", "Resume")
		w := ui.TextSize(label).X + 20
		if widgets.ButtonRect(ui, ui.ID("queue-resume"), mathx.Rect{X: right - w, Y: row.Y, W: w, H: row.H}, label) {
			p.queuePaused = false
		}
		right -= w + 6
	}
	label := i18n.Tr("editor.agent.queue_count", "Queued: {n}", i18n.Int("n", int64(len(p.queue))))
	ui.TextIn(mathx.Rect{X: row.X, Y: row.Y, W: max(0, right-row.X), H: row.H}, widgets.Elide(ui, label, max(0, right-row.X)), th.TextDim, core.AlignLeft)
	row.Y += row.H + th.Spacing
	for i := 0; i < min(len(p.queue), 3); i++ {
		item := p.queue[i]
		text := strings.TrimSpace(firstLine(item.text))
		if text == "" && len(item.images) > 0 {
			text = i18n.Tr("editor.agent.queue_image", "Image")
		}
		text = strconv.Itoa(i+1) + ". " + text
		remove := mathx.Rect{X: row.X + row.W - row.H, Y: row.Y, W: row.H, H: row.H}
		if widgets.ButtonRect(ui, core.IDSeq(ui.ID("queue-remove"), i), remove, "×") {
			p.removeQueued(i)
			return
		}
		ui.TextIn(mathx.Rect{X: row.X + 4, Y: row.Y, W: max(0, row.W-row.H-8), H: row.H}, widgets.Elide(ui, text, max(0, row.W-row.H-8)), th.TextDim, core.AlignLeft)
		row.Y += row.H + th.Spacing
	}
	if len(p.queue) > 3 {
		label := i18n.Tr("editor.agent.queue_more", "+{n} more queued", i18n.Int("n", int64(len(p.queue)-3)))
		ui.TextIn(row, label, th.TextDisabled, core.AlignLeft)
	}
}
