package play

import (
	"slices"
	"sort"
	"time"
	"unsafe"

	"aego/core/clock"
	"aego/core/rewind"
	"aego/engine"
	"aego/scene/sceneio"
)

const (
	DefaultKeyEvery = 300
	MinKeyEvery     = 30
	startKeyEvery   = 120
	spareKeys       = 2
	TargetReplay    = 1500 * time.Microsecond
	costAlpha       = 0.05
	TransientEvery  = 15
	TransientSpan   = 60
	TransientKeys   = 32
	TransientBytes  = 32 << 20
	transientSpare  = 4
)

// Age thresholds of key thinning (D359); variables so a test can shorten them.
var (
	RecentFrames = 2 * 60 * 60
	OldFrames    = 10 * 60 * 60
	MidSpacing   = uint64(300)
	OldSpacing   = uint64(1200)
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
	if c.keyEvery != 0 {
		return c.keyEvery
	}
	if c.frameCost <= 0 {
		return startKeyEvery
	}
	k := uint64(TargetReplay.Seconds() / c.frameCost)
	return min(max(k, MinKeyEvery), DefaultKeyEvery)
}

func (c *Controller) noteCost(d time.Duration) {
	per := d.Seconds()
	if c.frameCost == 0 {
		c.frameCost = per
		return
	}
	c.frameCost += (per - c.frameCost) * costAlpha
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

func (c *Controller) snap(k *snapshot, frame int) {
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
}

func (c *Controller) capture(frame int) {
	start := time.Now()
	var k *snapshot
	if n := len(c.spare); n > 0 {
		k, c.spare = c.spare[n-1], c.spare[:n-1]
	} else {
		k = &snapshot{}
	}
	c.snap(k, frame)
	c.keys = append(c.keys, k)
	c.keyBytes += k.size
	c.lastTick = k.tick
	c.keyTime = time.Since(start)
	c.thin()
	c.trim()
}

func spacing(frame, newest int) uint64 {
	switch age := newest - frame; {
	case age >= OldFrames:
		return OldSpacing
	case age >= RecentFrames:
		return MidSpacing
	}
	return 0
}

// thin drops keys closer than the spacing of their age to the key kept before them; the oldest and newest stay.
func (c *Controller) thin() {
	n := len(c.keys)
	if n < 3 {
		return
	}
	newest := c.keys[n-1].frame
	out := c.keys[:1]
	for i := 1; i < n; i++ {
		k := c.keys[i]
		if i < n-1 && k.tick-out[len(out)-1].tick < spacing(k.frame, newest) {
			c.releaseKey(k)
			continue
		}
		out = append(out, k)
	}
	clear(c.keys[len(out):])
	c.keys = out
}

func (c *Controller) hasKey(frame int) bool {
	for i := len(c.keys) - 1; i >= 0 && c.keys[i].frame >= frame; i-- {
		if c.keys[i].frame == frame {
			return true
		}
	}
	for _, k := range c.tkeys {
		if k.frame == frame {
			return true
		}
	}
	return false
}

func (c *Controller) wantTransient(to int) bool {
	d := to - c.cursor
	return d > 0 && d <= TransientSpan && d%TransientEvery == 0 && c.keysOn() && !c.hasKey(c.cursor)
}

func (c *Controller) keepTransient(target int) {
	var k *snapshot
	if n := len(c.tspare); n > 0 {
		k, c.tspare = c.tspare[n-1], c.tspare[:n-1]
	} else {
		k = &snapshot{}
	}
	c.snap(k, c.cursor)
	i := sort.Search(len(c.tkeys), func(i int) bool { return c.tkeys[i].frame > k.frame })
	c.tkeys = slices.Insert(c.tkeys, i, k)
	c.tbytes += k.size
	for len(c.tkeys) > TransientKeys || c.tbytes > TransientBytes {
		c.evictTransient(target)
	}
}

func (c *Controller) freeTransient(k *snapshot) {
	c.tbytes -= k.size
	if len(c.tspare) < transientSpare {
		c.tspare = append(c.tspare, k)
	}
}

// evictTransient drops the cached key farthest from the target.
func (c *Controller) evictTransient(target int) {
	far, at := -1, 0
	for i, k := range c.tkeys {
		d := k.frame - target
		if d < 0 {
			d = -d
		}
		if d > far {
			far, at = d, i
		}
	}
	k := c.tkeys[at]
	c.tkeys = slices.Delete(c.tkeys, at, at+1)
	c.freeTransient(k)
}

func (c *Controller) dropTransient(after int) {
	n := len(c.tkeys)
	for n > 0 && c.tkeys[n-1].frame > after {
		n--
		c.freeTransient(c.tkeys[n])
	}
	clear(c.tkeys[n:])
	c.tkeys = c.tkeys[:n]
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
	c.dropTransient(after)
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
	var best *snapshot
	for i := len(c.keys) - 1; i >= 0; i-- {
		if c.keys[i].frame <= n {
			best = c.keys[i]
			break
		}
	}
	for i := len(c.tkeys) - 1; i >= 0; i-- {
		if k := c.tkeys[i]; k.frame <= n {
			if best == nil || k.frame > best.frame {
				best = k
			}
			break
		}
	}
	return best
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

// KeyFrames lists the frames of the history keys, oldest first.
func (c *Controller) KeyFrames() []int {
	out := make([]int, len(c.keys))
	for i, k := range c.keys {
		out[i] = k.frame
	}
	return out
}
