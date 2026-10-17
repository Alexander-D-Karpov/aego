package vm

import "aego/core/handle"

type coTag struct{}

type CoHandle = handle.Handle[coTag]

type CoState uint8

const (
	CoRunning CoState = iota
	CoSuspended
	CoDone
	CoCanceled
	CoFaulted
)

type WaitKind uint8

const (
	WaitNext WaitKind = iota
	WaitTime
	WaitFrames
	WaitFixed
	WaitAwait
	WaitUnscaled
)

type CoHooks interface {
	Yielded(h CoHandle, w WaitKind, arg Value)
	Ended(h CoHandle, st CoState)
}

type coroutine struct {
	gen      uint32
	live     bool
	canceled bool
	fn       *Func
	pc       int
	regs     []Value
	owner    InstanceID
	state    CoState
}

func CoValue(h CoHandle) Value {
	if h.IsZero() {
		return Value{}
	}
	return Value{A: uint64(h.Gen)<<32 | uint64(h.Index)}
}

func CoOf(v Value) CoHandle {
	return CoHandle{Index: uint32(v.A), Gen: uint32(v.A >> 32)}
}

func (m *Machine) co(h CoHandle) *coroutine {
	if h.Gen == 0 || int(h.Index) >= len(m.cos) {
		return nil
	}
	c := &m.cos[h.Index]
	if !c.live || c.gen != h.Gen {
		return nil
	}
	return c
}

func (m *Machine) newCo(f *Func, owner InstanceID) CoHandle {
	var idx uint32
	if n := len(m.cofree); n > 0 {
		idx = m.cofree[n-1]
		m.cofree = m.cofree[:n-1]
	} else {
		m.cos = append(m.cos, coroutine{})
		idx = uint32(len(m.cos) - 1)
	}
	c := &m.cos[idx]
	c.gen++
	if c.gen == 0 {
		c.gen = 1
	}
	c.live, c.canceled, c.fn, c.pc, c.owner, c.state = true, false, f, 0, owner, CoRunning
	if cap(c.regs) < f.Regs {
		c.regs = make([]Value, f.Regs)
	} else {
		c.regs = c.regs[:f.Regs]
	}
	m.colive++
	return CoHandle{Index: idx, Gen: c.gen}
}

func (m *Machine) endCo(idx uint32, st CoState) {
	c := &m.cos[idx]
	h := CoHandle{Index: idx, Gen: c.gen}
	c.live, c.fn, c.state, c.canceled = false, nil, st, false
	clear(c.regs)
	m.cofree = append(m.cofree, idx)
	m.colive--
	if m.Co != nil {
		m.Co.Ended(h, st)
	}
}

func (m *Machine) yieldCo(idx uint32, ret, pc int, regs []Value, w WaitKind, arg Value) {
	c := &m.cos[idx]
	h := CoHandle{Index: idx, Gen: c.gen}
	if c.canceled {
		m.endCo(idx, CoCanceled)
	} else {
		c.pc = pc
		copy(c.regs, regs)
		c.state = CoSuspended
		if m.Co != nil {
			m.Co.Yielded(h, w, arg)
		}
	}
	if ret >= 0 {
		m.stack[ret] = CoValue(h)
	}
}

func (m *Machine) finishCo(idx uint32, ret int) {
	c := &m.cos[idx]
	h := CoHandle{Index: idx, Gen: c.gen}
	st := CoDone
	if c.canceled {
		st = CoCanceled
	}
	m.endCo(idx, st)
	if ret >= 0 {
		m.stack[ret] = CoValue(h)
	}
}

func (m *Machine) Start(f *Func, self []Value, args []Value, owner InstanceID) (CoHandle, *Fault) {
	if len(m.frames) != 0 {
		return CoHandle{}, &Fault{Code: CodeLink, Message: "the machine is already running a call"}
	}
	if f == nil || !f.Coroutine || len(args) != f.Params {
		return CoHandle{}, &Fault{Code: CodeLink, Message: "function is not a coroutine or the argument count does not match"}
	}
	if f.Regs > len(m.stack) {
		m.grow(f.Regs)
	}
	copy(m.stack, args)
	clear(m.stack[len(args):f.Regs])
	h := m.newCo(f, owner)
	m.left = m.opt.Budget
	m.frames = append(m.frames, frame{f: f, ret: -1, self: self, owner: owner, co: h.Index + 1})
	m.stats.Calls++
	if fl := m.finish(); fl != nil {
		return CoHandle{}, fl
	}
	return h, nil
}

func (m *Machine) Resume(h CoHandle, self []Value) (bool, *Fault) {
	if len(m.frames) != 0 {
		return false, &Fault{Code: CodeLink, Message: "the machine is already running a call"}
	}
	c := m.co(h)
	if c == nil || c.state != CoSuspended {
		return true, nil
	}
	f := c.fn
	if f.Regs > len(m.stack) {
		m.grow(f.Regs)
	}
	copy(m.stack, c.regs[:f.Regs])
	c.state = CoRunning
	m.left = m.opt.Budget
	m.frames = append(m.frames, frame{f: f, pc: c.pc, ret: -1, self: self, owner: c.owner, co: h.Index + 1})
	m.stats.Calls++
	if fl := m.finish(); fl != nil {
		return true, fl
	}
	c = m.co(h)
	return c == nil || c.state != CoSuspended, nil
}

func (m *Machine) Cancel(h CoHandle) {
	c := m.co(h)
	switch {
	case c == nil:
	case c.state == CoRunning:
		c.canceled = true
	default:
		m.endCo(h.Index, CoCanceled)
	}
}

func (m *Machine) CoState(h CoHandle) (CoState, bool) {
	c := m.co(h)
	if c == nil {
		return CoDone, false
	}
	if c.canceled {
		return CoCanceled, true
	}
	return c.state, true
}

func (m *Machine) CoOwner(h CoHandle) (InstanceID, bool) {
	c := m.co(h)
	if c == nil {
		return NoInstance, false
	}
	return c.owner, true
}

func (m *Machine) CoRunning(v Value) bool {
	c := m.co(CoOf(v))
	return c != nil && !c.canceled
}

func (m *Machine) unwind(floor int) {
	for i := len(m.frames) - 1; i >= floor; i-- {
		if c := m.frames[i].co; c != 0 {
			m.endCo(c-1, CoFaulted)
		}
	}
	m.frames = m.frames[:floor]
}
