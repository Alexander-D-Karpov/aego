package play

import (
	"strconv"

	"aego/core/errs"
	"aego/editor/core/context"
	"aego/engine"
)

func (c *Controller) SetBlocked(why string) {
	c.blocked = why
}

func (c *Controller) SetHistoryLimit(bytes int) {
	c.limit = bytes
}

func (c *Controller) Taint(why string) {
	if c.hist != nil && !c.replaying {
		c.hist.cutAt(c.cursor, why)
	}
}

// Invalidate drops every recorded frame: a reboot would replay them with the changed code, assets or settings.
func (c *Controller) Invalidate(why string) {
	if c.hist != nil && !c.replaying {
		c.hist.cutAt(0, why)
	}
}

func (c *Controller) Timeline() Timeline {
	tl := Timeline{Blocked: c.blocked}
	h := c.hist
	if h == nil || c.state == Stopped {
		return tl
	}
	tl.Frames, tl.Cursor, tl.Usable, tl.Why, tl.Beyond = len(h.frames), c.cursor, h.usable(), h.why, c.beyond
	if c.cursor > 0 {
		tl.Tick = h.frames[c.cursor-1].tick
	}
	return tl
}

func (c *Controller) watchEdits() {
	if c.editUnsub != nil {
		c.editUnsub()
		c.editUnsub = nil
	}
	if c.proxy != nil {
		c.editUnsub = c.proxy.History.Subscribe(func() { c.Taint("the scene was edited during Play") })
	}
}

func (c *Controller) record(dt float64) bool {
	h := c.hist
	if c.cursor < len(h.frames) {
		h.truncate(c.cursor)
	}
	if rev := c.ctx.AssetRev; rev != c.assetRev {
		c.assetRev = rev
		h.cutAt(0, "assets were reloaded during Play")
	}
	t := c.world.Time
	ok := h.add(dt, t.Scale(), t.Paused(), c.stepReq, &c.input)
	c.stepReq = stepNone
	c.cursor = len(h.frames)
	c.beyond = !ok
	return ok
}

func (c *Controller) CanSeekBack() bool {
	return c.state != Stopped && c.hist != nil && c.blocked == "" && (c.cursor > 0 || c.beyond)
}

func (c *Controller) StepBack() error {
	n := c.cursor - 1
	if c.beyond {
		n = c.hist.usable()
	}
	return c.Seek(n)
}

func (c *Controller) StepForward() error {
	if c.hist != nil && c.cursor < len(c.hist.frames) {
		return c.Seek(c.cursor + 1)
	}
	if c.state == Playing {
		c.Pause()
	}
	c.Step()
	return nil
}

func (c *Controller) Seek(n int) (err error) {
	h := c.hist
	switch {
	case h == nil || c.state == Stopped:
		return errs.New(errs.EditorCommand, "rewind works during Play")
	case c.blocked != "":
		return errs.New(errs.EditorCommand, c.blocked)
	}
	n = min(max(n, 0), h.usable())
	defer c.guard("rewind")
	if n < c.cursor || c.beyond {
		if err := c.reboot(); err != nil {
			return err
		}
	}
	c.replay(n)
	c.pauseHere()
	return nil
}

func (c *Controller) reboot() error {
	h := c.hist
	view := c.proxy.View
	c.teardown()
	c.world, c.runner, c.scene = nil, nil, nil
	if err := c.start(h.data, h.name); err != nil {
		c.Stop()
		return err
	}
	real := c.ctx.Docs.Get(c.doc)
	if real == nil {
		c.Stop()
		return errs.New(errs.EditorDocument, "scene is not open")
	}
	c.proxy = real.PlayCopy(c.scene)
	c.proxy.View = view
	c.ctx.Docs.SetPlay(c.proxy, c.world)
	c.watchEdits()
	c.cursor, c.beyond = 0, false
	c.assetRev = c.ctx.AssetRev
	return nil
}

// replayBreak fires on the same Break call of the frame as the recorded run did.
func (c *Controller) replayBreak(f *frameRec) func() bool {
	calls := int32(0)
	return func() bool {
		calls++
		return calls == f.brk
	}
}

func (c *Controller) replay(to int) {
	h, w, e := c.hist, c.world, c.ctx.Engine
	brk := w.Break
	c.replaying = true
	for c.cursor < to {
		f := h.frames[c.cursor]
		t := w.Time
		t.SetScale(f.scale)
		t.SetPaused(f.paused)
		switch f.step {
		case stepFixed:
			t.StepFixed()
		case stepFrame:
			t.StepFrame()
		}
		c.input = h.inputs[f.input]
		w.Break = c.replayBreak(&f)
		e.TickWorld(w, engine.ScopePlay, f.dt)
		c.cursor++
	}
	c.replaying = false
	w.Break = brk
}

func (c *Controller) pauseHere() {
	t := c.world.Time
	t.SetScale(c.scale)
	t.SetPaused(true)
	c.stats.Frame, c.stats.Updates, c.stats.Tick, c.stats.Alpha = t.Frame(), t.Updates(), t.Tick(), t.Alpha()
	if c.state != Paused {
		c.state = Paused
		c.notify()
	}
	c.ctx.Notify(context.LevelInfo, "Rewound to tick "+strconv.FormatUint(t.Tick(), 10))
}
