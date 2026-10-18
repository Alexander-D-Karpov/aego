package play

import (
	"time"
	"unsafe"

	"aego/core/clock"
	"aego/core/rewind"
	"aego/engine"
	"aego/scene/sceneio"
)

const (
	DefaultKeyEvery = 300
	spareKeys       = 2
)

type snapshot struct {
	frame int
	tick  uint64
	time  clock.TimeState
	rng   uint64
	keys  []rewind.Key
	size  int
}

func (c *Controller) SetKeyEvery(ticks uint64) {
	c.keyEvery = ticks
}

func (c *Controller) every() uint64 {
	if c.keyEvery == 0 {
		return DefaultKeyEvery
	}
	return c.keyEvery
}

func (c *Controller) keysOn() bool {
	return c.kfOff == "" && len(c.kf) > 0
}

func (c *Controller) bindKeys() {
	c.dropKeys(-1)
	c.kf, c.kfOff, c.floor, c.lastTick = c.kf[:0], "", 0, 0
	if c.world == nil {
		return
	}
	c.world.EachOwner(func(id string, v any) {
		k, ok := rewind.Of(v)
		switch {
		case ok:
			c.kf = append(c.kf, k)
		case c.kfOff == "":
			c.kfOff = id
		}
	})
}

func (c *Controller) capture(frame int) {
	start := time.Now()
	var k *snapshot
	if n := len(c.spare); n > 0 {
		k, c.spare = c.spare[n-1], c.spare[:n-1]
	} else {
		k = &snapshot{}
	}
	w := c.world
	k.frame, k.tick = frame, w.Time.Tick()
	w.Time.Capture(&k.time)
	k.rng = w.Random.State()
	for len(k.keys) < len(c.kf) {
		k.keys = append(k.keys, nil)
	}
	k.keys = k.keys[:len(c.kf)]
	k.size = int(unsafe.Sizeof(*k))
	for i, f := range c.kf {
		k.keys[i] = f.Capture(k.keys[i])
		k.size += k.keys[i].Size()
	}
	c.keys = append(c.keys, k)
	c.keyBytes += k.size
	c.lastTick = k.tick
	c.keyTime = time.Since(start)
	c.trim()
}

func (c *Controller) releaseKey(k *snapshot) {
	c.keyBytes -= k.size
	if len(c.spare) < spareKeys {
		c.spare = append(c.spare, k)
	}
}

func (c *Controller) trim() {
	h := c.hist
	n := 0
	for len(c.keys)-n > 1 && h.bytes+c.keyBytes > h.limit {
		c.releaseKey(c.keys[n])
		n++
	}
	if n > 0 {
		m := copy(c.keys, c.keys[n:])
		clear(c.keys[m:])
		c.keys = c.keys[:m]
	}
	if len(c.keys) > 0 {
		c.floor = c.keys[0].frame
	}
}

func (c *Controller) dropKeys(after int) {
	n := len(c.keys)
	for n > 0 && c.keys[n-1].frame > after {
		n--
		c.releaseKey(c.keys[n])
	}
	clear(c.keys[n:])
	c.keys = c.keys[:n]
	c.lastTick = 0
	if n > 0 {
		c.lastTick = c.keys[n-1].tick
		return
	}
	c.floor = 0
}

func (c *Controller) keyAt(n int) *snapshot {
	for i := len(c.keys) - 1; i >= 0; i-- {
		if c.keys[i].frame <= n {
			return c.keys[i]
		}
	}
	return nil
}

func (c *Controller) restore(k *snapshot) {
	w := c.world
	w.Time.Restore(&k.time)
	w.Random.SetState(k.rng)
	for i, f := range c.kf {
		f.Restore(k.keys[i])
	}
	for _, f := range c.kf {
		if a, ok := f.(rewind.AfterRestorer); ok {
			a.AfterRestore()
		}
	}
	c.cursor, c.beyond = k.frame, false
}

func (c *Controller) advance(dt float64) clock.Plan {
	plan := c.ctx.Engine.TickWorld(c.world, engine.ScopePlay, dt)
	if c.scene != nil {
		c.scene.Flush()
	}
	return plan
}

func (c *Controller) setReplaying(on bool) {
	if w := c.world; w != nil {
		w.SetReplaying(on)
		w.Recorder.SetReplaying(on)
	}
}

func (c *Controller) applyRebase() {
	if !c.rebase || c.hist == nil || c.scene == nil {
		return
	}
	c.rebase = false
	h := c.hist
	c.scene.Flush()
	h.data = sceneio.Save(c.scene)
	h.truncate(0)
	h.end, h.why = -1, ""
	c.cursor, c.beyond = 0, false
	c.dropKeys(-1)
	if c.keysOn() {
		c.capture(0)
	}
}

func (c *Controller) StateHash() [32]byte {
	if c.world == nil {
		return [32]byte{}
	}
	if c.hasher == nil {
		c.hasher = rewind.NewHasher()
	}
	h := c.hasher
	h.Reset()
	c.world.Time.Hash(h)
	h.U64(c.world.Random.State())
	for _, f := range c.kf {
		f.Hash(h)
	}
	return h.Sum()
}
