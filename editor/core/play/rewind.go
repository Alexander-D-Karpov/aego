package play

import (
	"strconv"

	"aego/core/errs"
	"aego/editor/core/context"
)

func (c *Controller) SetBlocked(why string) {
	c.blocked = why
}

func (c *Controller) SetHistoryLimit(bytes int) {
	c.limit = bytes
}

func (c *Controller) Taint(why string) {
	if c.hist == nil || c.replaying {
		return
	}
	if c.cursor == 0 {
		c.rebase = true
		return
	}
	c.hist.cutAt(c.cursor, why)
	c.dropKeys(c.cursor)
}

// Invalidate drops every recorded frame: a reboot would replay them with the changed code, assets or settings.
func (c *Controller) Invalidate(why string) {
	if c.hist == nil || c.replaying {
		return
	}
	c.hist.cutAt(0, why)
	c.dropKeys(-1)
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
	tl.Floor, tl.Keys, tl.KeyBytes, tl.KeysOff = c.floor, len(c.keys), c.keyBytes, c.kfOff
	tl.KeyTime, tl.KeyEvery = c.keyTime, c.every()
	tl.Cached, tl.CacheBytes, tl.Bytes = len(c.tkeys), c.tbytes, h.bytes
	if n := len(c.keys); n > 0 {
		tl.LastKey = c.keys[n-1].size
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
	if len(c.tkeys) > 0 {
		c.dropTransient(-1)
	}
	if c.cursor < len(h.frames) {
		h.truncate(c.cursor)
		c.dropKeys(c.cursor)
	}
	if rev := c.ctx.AssetRev; rev != c.assetRev {
		c.assetRev = rev
		h.cutAt(0, "assets were reloaded during Play")
		c.dropKeys(-1)
	}
	t := c.world.Time
	ok := h.add(dt, t.Scale(), t.Paused(), c.stepReq, &c.input)
	c.stepReq = stepNone
	c.cursor = len(h.frames)
	c.beyond = !ok
	return ok
}

func (c *Controller) Replayed() int {
	return c.replayed
}

func (c *Controller) CanSeekBack() bool {
	return c.state != Stopped && c.hist != nil && c.blocked == "" && (c.cursor > c.floor || c.beyond)
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

func (c *Controller) Seek(n int) error {
	return c.seek(n, true)
}

// Scrub seeks without a notice, for dragging the timeline.
func (c *Controller) Scrub(n int) error {
	return c.seek(n, false)
}

func (c *Controller) seek(n int, notice bool) error {
	switch {
	case c.hist == nil || c.state == Stopped:
		return errs.New(errs.EditorCommand, "rewind works during Play")
	case c.blocked != "":
		return errs.New(errs.EditorCommand, c.blocked)
	}
	defer c.guard("rewind")
	c.applyRebase()
	n = min(max(n, c.floor), c.hist.usable())
	c.setReplaying(true)
	defer c.setReplaying(false)
	if err := c.rewindTo(n); err != nil {
		return err
	}
	c.replay(n)
	c.pauseHere(notice)
	return nil
}

func (c *Controller) rewindTo(n int) error {
	if c.keysOn() {
		k := c.keyAt(max(n-1, 0))
		switch {
		case k == nil:
			return c.reboot()
		case c.beyond || n < c.cursor || k.frame > c.cursor:
			c.restore(k)
		}
		return nil
	}
	if n < c.cursor || c.beyond {
		return c.reboot()
	}
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
	if h.end == 0 {
		h.truncate(0)
		h.end, h.why = -1, ""
	}
	c.bindKeys()
	if c.keysOn() {
		c.capture(0)
	}
	c.setReplaying(true)
	return nil
}

func (c *Controller) replayCall() bool {
	c.rcalls++
	return c.rcalls == c.rbrk
}

// replay runs the recorded frames up to to, keeping transient keys near the target for scrubbing (D346).
func (c *Controller) replay(to int) {
	h, w := c.hist, c.world
	brk := w.Break
	c.replaying = true
	c.replayed = max(to-c.cursor, 0)
	w.Break = c.replayFn
	for c.cursor < to {
		f := &h.frames[c.cursor]
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
		c.rcalls, c.rbrk = 0, f.brk
		c.advance(f.dt)
		c.cursor++
		if c.wantTransient(to) {
			c.keepTransient(to)
		}
	}
	c.replaying = false
	w.Break = brk
}

func (c *Controller) pauseHere(notice bool) {
	t := c.world.Time
	t.SetScale(c.scale)
	t.SetPaused(true)
	c.held = true
	c.stats.Frame, c.stats.Updates, c.stats.Tick, c.stats.Alpha = t.Frame(), t.Updates(), t.Tick(), t.Alpha()
	if c.state != Paused {
		c.state = Paused
		c.notify()
	}
	if notice {
		c.ctx.Notify(context.LevelInfo, "Rewound to tick "+strconv.FormatUint(t.Tick(), 10))
	}
}
