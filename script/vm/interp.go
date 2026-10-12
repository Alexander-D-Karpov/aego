package vm

import (
	"math"
	"strconv"

	"aego/script/bytecode"
)

func (m *Machine) at(top int) (*frame, *Func, int, []bytecode.Instr, []Value) {
	fr := &m.frames[top]
	return fr, fr.f, fr.pc, fr.f.code, m.stack[fr.base : fr.base+fr.f.Regs]
}

func (m *Machine) enter(top, pc int, callee *Func, nb, ret int, self []Value, owner InstanceID) bool {
	if len(m.frames) >= m.opt.MaxCallDepth {
		m.trap(top, pc, CodeCallDepth, "call depth exceeds "+strconv.Itoa(m.opt.MaxCallDepth))
		return false
	}
	if need := nb + callee.Regs; need > len(m.stack) {
		m.grow(need)
	}
	clear(m.stack[nb+callee.Params : nb+callee.Regs])
	m.frames[top].pc = pc
	m.frames = append(m.frames, frame{f: callee, base: nb, ret: ret, self: self, owner: owner})
	m.stats.Calls++
	return true
}

func (m *Machine) run(floor int) {
	top := len(m.frames) - 1
	fr, f, pc, code, regs := m.at(top)
	for {
		if m.left == 0 {
			m.stats.BudgetAborts++
			m.trap(top, pc+1, CodeBudget, "instruction budget exceeded")
			return
		}
		m.left--
		in := code[pc]
		pc++
		a, b, c := int(in.A()), int(in.B()), int(in.C())
		switch in.Op() {
		case bytecode.NOP:
		case bytecode.MOVE:
			regs[a] = regs[b]
		case bytecode.MOVEN:
			copy(regs[a:a+c], regs[b:b+c])
		case bytecode.LOADK:
			regs[a] = f.consts[in.Bx()]
		case bytecode.LOADI:
			regs[a] = Int(int64(in.SBx()))
		case bytecode.LOADNIL:
			regs[a] = Value{}
		case bytecode.LOADBOOL:
			regs[a] = Bool(b != 0)
		case bytecode.ADDI:
			regs[a] = Int(regs[b].Int() + regs[c].Int())
		case bytecode.SUBI:
			regs[a] = Int(regs[b].Int() - regs[c].Int())
		case bytecode.MULI:
			regs[a] = Int(regs[b].Int() * regs[c].Int())
		case bytecode.DIVI, bytecode.MODI:
			x, y := regs[b].Int(), regs[c].Int()
			switch {
			case y == 0:
				m.trap(top, pc, CodeDivZero, "integer division by zero")
				return
			case y == -1 && in.Op() == bytecode.DIVI:
				regs[a] = Int(-x)
			case y == -1:
				regs[a] = Value{}
			case in.Op() == bytecode.DIVI:
				regs[a] = Int(x / y)
			default:
				regs[a] = Int(x % y)
			}
		case bytecode.NEGI:
			regs[a] = Int(-regs[b].Int())
		case bytecode.ADDF:
			regs[a] = Float(regs[b].Float() + regs[c].Float())
		case bytecode.SUBF:
			regs[a] = Float(regs[b].Float() - regs[c].Float())
		case bytecode.MULF:
			regs[a] = Float(regs[b].Float() * regs[c].Float())
		case bytecode.DIVF:
			regs[a] = Float(regs[b].Float() / regs[c].Float())
		case bytecode.NEGF:
			regs[a] = Float(-regs[b].Float())
		case bytecode.ITOF:
			regs[a] = Float(float32(regs[b].Int()))
		case bytecode.FTOI:
			regs[a] = Int(ftoi(regs[b].Float()))
		case bytecode.ITOS:
			regs[a] = m.NewString(strconv.FormatInt(regs[b].Int(), 10))
		case bytecode.FTOS:
			regs[a] = m.NewString(strconv.FormatFloat(float64(regs[b].Float()), 'g', -1, 32))
		case bytecode.BTOS:
			if regs[b].Bool() {
				regs[a] = m.Intern("true")
			} else {
				regs[a] = m.Intern("false")
			}
		case bytecode.ADDS:
			x, ok1 := m.Str(regs[b])
			y, ok2 := m.Str(regs[c])
			switch {
			case !ok1 || !ok2:
				m.trap(top, pc, CodeBadHandle, "string handle is not valid")
				return
			case x == "":
				regs[a] = regs[c]
			case y == "":
				regs[a] = regs[b]
			default:
				regs[a] = m.NewString(x + y)
			}
		case bytecode.LENS:
			s, ok := m.Str(regs[b])
			if !ok {
				m.trap(top, pc, CodeBadHandle, "string handle is not valid")
				return
			}
			regs[a] = Int(int64(len(s)))
		case bytecode.EQ:
			regs[a] = Bool(regs[b] == regs[c])
		case bytecode.EQF:
			regs[a] = Bool(regs[b].Float() == regs[c].Float())
		case bytecode.EQS, bytecode.LTS, bytecode.LES:
			x, ok1 := m.Str(regs[b])
			y, ok2 := m.Str(regs[c])
			if !ok1 || !ok2 {
				m.trap(top, pc, CodeBadHandle, "string handle is not valid")
				return
			}
			switch in.Op() {
			case bytecode.EQS:
				regs[a] = Bool(x == y)
			case bytecode.LTS:
				regs[a] = Bool(x < y)
			default:
				regs[a] = Bool(x <= y)
			}
		case bytecode.EQV:
			x, y := regs[b], regs[c]
			regs[a] = Bool(x.Lane(0) == y.Lane(0) && x.Lane(1) == y.Lane(1) && x.Lane(2) == y.Lane(2) && x.Lane(3) == y.Lane(3))
		case bytecode.LTI:
			regs[a] = Bool(regs[b].Int() < regs[c].Int())
		case bytecode.LEI:
			regs[a] = Bool(regs[b].Int() <= regs[c].Int())
		case bytecode.LTF:
			regs[a] = Bool(regs[b].Float() < regs[c].Float())
		case bytecode.LEF:
			regs[a] = Bool(regs[b].Float() <= regs[c].Float())
		case bytecode.NOT:
			regs[a] = Bool(!regs[b].Bool())
		case bytecode.JMP:
			pc += int(in.SBx())
		case bytecode.JMPIF:
			if regs[a].A != 0 {
				pc += int(in.SBx())
			}
		case bytecode.JMPNOT:
			if regs[a].A == 0 {
				pc += int(in.SBx())
			}
		case bytecode.CALL, bytecode.CALLX:
			var callee *Func
			if in.Op() == bytecode.CALL {
				callee = f.mod.Funcs[in.Bx()]
			} else if callee = f.mod.externs[in.Bx()]; callee == nil {
				m.trap(top, pc, CodeLink, "extern function is not linked")
				return
			}
			if !m.enter(top, pc, callee, fr.base+a, fr.base+a, fr.self, fr.owner) {
				return
			}
			top++
			fr, f, pc, code, regs = m.at(top)
		case bytecode.CALLR:
			x := &f.mod.xmethods[in.Bx()]
			if m.Scripts == nil {
				m.trap(top, pc, CodeLink, "script references are not bound")
				return
			}
			tgt, id, ok := m.Scripts.Instance(regs[a], x.script)
			if !ok {
				m.trap(top, pc, CodeNil, "script reference is nil or its instance is gone")
				return
			}
			if !m.enter(top, pc, x.fn, fr.base+a+1, fr.base+a, tgt, id) {
				return
			}
			top++
			fr, f, pc, code, regs = m.at(top)
		case bytecode.START:
			callee := f.mod.Funcs[in.Bx()]
			if !m.enter(top, pc, callee, fr.base+a, fr.base+a, fr.self, fr.owner) {
				return
			}
			h := m.newCo(callee, fr.owner)
			top++
			m.frames[top].co = h.Index + 1
			fr, f, pc, code, regs = m.at(top)
		case bytecode.CALLH:
			bx := int(in.Bx())
			m.frames[top].pc = pc
			m.stats.HostCalls++
			if err := f.mod.hosts[bx](m, regs[a:a+f.mod.windows[bx]]); err != nil {
				if m.fault == nil {
					m.trapErr(top, pc, err)
				}
				return
			}
		case bytecode.RET, bytecode.YIELD:
			switch {
			case in.Op() == bytecode.YIELD && fr.co == 0:
				m.trap(top, pc, CodeLink, "yield outside a coroutine frame")
				return
			case in.Op() == bytecode.YIELD:
				arg := Value{}
				if c != int(WaitNext) {
					arg = regs[a]
				}
				m.yieldCo(fr.co-1, fr.ret, pc, regs, WaitKind(c), arg)
			case fr.co != 0:
				m.finishCo(fr.co-1, fr.ret)
			default:
				copy(m.stack[fr.ret:fr.ret+b], regs[a:a+b])
			}
			m.frames = m.frames[:top]
			if top == floor {
				return
			}
			top--
			fr, f, pc, code, regs = m.at(top)
		case bytecode.GETSF, bytecode.SETSF:
			bx := int(in.Bx())
			self := fr.self
			if bx >= len(self) {
				m.trap(top, pc, CodeLink, "script field is outside the instance")
				return
			}
			if in.Op() == bytecode.GETSF {
				regs[a] = self[bx]
			} else {
				self[bx] = regs[a]
			}
		case bytecode.GETCF, bytecode.SETCF:
			ref := regs[b]
			if in.Op() == bytecode.SETCF {
				ref = regs[a]
			}
			switch {
			case m.Comps == nil:
				m.trap(top, pc, CodeLink, "component access is not bound")
				return
			case ref.IsZero():
				m.trap(top, pc, CodeNil, "component reference is nil")
				return
			}
			var err error
			if in.Op() == bytecode.GETCF {
				err = m.Comps.GetField(ref, c, &regs[a])
			} else {
				err = m.Comps.SetField(ref, c, regs[b])
			}
			if err != nil {
				m.trap(top, pc, CodeNil, err.Error())
				return
			}
		case bytecode.GETXF, bytecode.SETXF:
			x := &f.mod.xfields[c]
			ref, val := regs[b], a
			if in.Op() == bytecode.SETXF {
				ref, val = regs[a], b
			}
			if m.Scripts == nil {
				m.trap(top, pc, CodeLink, "script references are not bound")
				return
			}
			tgt, _, ok := m.Scripts.Instance(ref, x.script)
			if !ok || x.slot+x.width > len(tgt) {
				m.trap(top, pc, CodeNil, "script reference is nil or its instance is gone")
				return
			}
			if in.Op() == bytecode.GETXF {
				copy(regs[val:val+x.width], tgt[x.slot:x.slot+x.width])
			} else {
				copy(tgt[x.slot:x.slot+x.width], regs[val:val+x.width])
			}
		case bytecode.GETSCRIPT:
			if m.Scripts == nil {
				m.trap(top, pc, CodeLink, "script references are not bound")
				return
			}
			if regs[a].IsZero() {
				m.trap(top, pc, CodeNil, "node reference is nil")
				return
			}
			ref, err := m.Scripts.Find(regs[a], f.mod.src.ScriptRefs[in.Bx()])
			if err != nil {
				m.trap(top, pc, CodeNil, err.Error())
				return
			}
			regs[a] = ref
		case bytecode.SELFREF:
			if len(fr.self) == 0 {
				m.trap(top, pc, CodeLink, "self reference outside a script instance")
				return
			}
			regs[a] = Value{A: fr.self[0].A, B: uint64(fr.owner)}
		case bytecode.COSTOP:
			if !regs[a].IsZero() {
				m.Cancel(CoOf(regs[a]))
			}
		case bytecode.CORUN:
			regs[a] = Bool(m.CoRunning(regs[b]))
		case bytecode.EMIT:
			if m.Signals == nil {
				m.trap(top, pc, CodeLink, "signals are not bound")
				return
			}
			n := f.mod.sigSlots[f.Script][in.Bx()]
			m.frames[top].pc = pc
			if err := m.Signals.Emit(fr.owner, int(in.Bx()), regs[a:a+n]); err != nil {
				m.trapErr(top, pc, err)
				return
			}
			if m.fault != nil {
				return
			}
			fr, f, pc, code, regs = m.at(top)
		case bytecode.CONNECT:
			if m.Signals == nil {
				m.trap(top, pc, CodeLink, "signals are not bound")
				return
			}
			v, err := m.Signals.Connect(regs[a+1], regs[a+2], &f.mod.connects[in.Bx()])
			if err != nil {
				m.trap(top, pc, CodeNil, err.Error())
				return
			}
			regs[a] = v
		case bytecode.DISCONNECT:
			if !regs[a].IsZero() && m.Signals != nil {
				m.Signals.Disconnect(regs[a])
			}
		case bytecode.CONNECTED:
			regs[a] = Bool(!regs[b].IsZero() && m.Signals != nil && m.Signals.Connected(regs[b]))
		case bytecode.NEWSLICE:
			t := f.mod.types[in.Bx()]
			regs[a] = m.heap.newSlice(t.width, t.mask, 4)
		case bytecode.ENSURE:
			if regs[a].A == 0 {
				t := f.mod.types[in.Bx()]
				regs[a] = m.heap.newSlice(t.width, t.mask, 4)
			}
		case bytecode.LEN:
			regs[a] = Int(int64(regs[b].B))
		case bytecode.INDEX, bytecode.SETINDEX:
			s, i, at := regs[b], regs[c].Int(), a
			if in.Op() == bytecode.SETINDEX {
				s, i, at = regs[a], regs[b].Int(), c
			}
			if i < 0 || uint64(i) >= s.B {
				m.trap(top, pc, CodeIndex, "index "+strconv.FormatInt(i, 10)+" out of range for length "+strconv.FormatUint(s.B, 10))
				return
			}
			o := m.heap.get(s.A)
			if o == nil || o.str {
				m.trap(top, pc, CodeBadHandle, "slice handle is not valid")
				return
			}
			w := o.width
			off := int(i) * w
			if at+w > len(regs) || off+w > len(o.elems) {
				m.trap(top, pc, CodeLink, "slice element does not fit the registers")
				return
			}
			if in.Op() == bytecode.INDEX {
				copy(regs[at:at+w], o.elems[off:off+w])
			} else {
				copy(o.elems[off:off+w], regs[at:at+w])
			}
		case bytecode.APPEND:
			s := regs[b]
			o := m.heap.get(s.A)
			if o == nil || o.str {
				m.trap(top, pc, CodeNil, "append to a nil slice")
				return
			}
			w := o.width
			n := int(s.B) * w
			if c+w > len(regs) || n > len(o.elems) {
				m.trap(top, pc, CodeLink, "slice element does not fit the registers")
				return
			}
			if n == len(o.elems) {
				before := cap(o.elems)
				o.elems = append(o.elems, regs[c:c+w]...)
				if d := cap(o.elems) - before; d > 0 {
					m.heap.grow(o, d*16)
				}
				regs[a] = Value{A: s.A, B: s.B + 1}
				break
			}
			width, mask := o.width, o.mask
			nv := m.heap.newSlice(width, mask, int(s.B)+1)
			old, no := m.heap.get(s.A), m.heap.get(nv.A)
			no.elems = append(no.elems, old.elems[:n]...)
			no.elems = append(no.elems, regs[c:c+w]...)
			regs[a] = Value{A: nv.A, B: s.B + 1}
		case bytecode.VEC2:
			regs[a] = Vec(regs[b].Float(), regs[b+1].Float(), 0, 0)
		case bytecode.VEC3:
			regs[a] = Vec(regs[b].Float(), regs[b+1].Float(), regs[b+2].Float(), 0)
		case bytecode.VEC4:
			regs[a] = Vec(regs[b].Float(), regs[b+1].Float(), regs[b+2].Float(), regs[b+3].Float())
		case bytecode.GETL:
			regs[a] = Float(regs[b].Lane(c))
		case bytecode.SETL:
			regs[a] = regs[a].WithLane(c, regs[b].Float())
		case bytecode.VADD:
			x, y := regs[b], regs[c]
			regs[a] = Vec(x.Lane(0)+y.Lane(0), x.Lane(1)+y.Lane(1), x.Lane(2)+y.Lane(2), x.Lane(3)+y.Lane(3))
		case bytecode.VSUB:
			x, y := regs[b], regs[c]
			regs[a] = Vec(x.Lane(0)-y.Lane(0), x.Lane(1)-y.Lane(1), x.Lane(2)-y.Lane(2), x.Lane(3)-y.Lane(3))
		case bytecode.VMULS:
			x, s := regs[b], regs[c].Float()
			regs[a] = Vec(x.Lane(0)*s, x.Lane(1)*s, x.Lane(2)*s, x.Lane(3)*s)
		case bytecode.VDIVS:
			x, s := regs[b], regs[c].Float()
			regs[a] = Vec(x.Lane(0)/s, x.Lane(1)/s, x.Lane(2)/s, x.Lane(3)/s)
		case bytecode.VNEG:
			x := regs[b]
			regs[a] = Vec(-x.Lane(0), -x.Lane(1), -x.Lane(2), -x.Lane(3))
		case bytecode.ISNIL:
			regs[a] = Bool(regs[b].IsZero())
		case bytecode.FAULT:
			msg, _ := m.Str(f.consts[in.Bx()])
			m.trap(top, pc, CodeExplicit, msg)
			return
		default:
			m.trap(top, pc, CodeLink, "invalid opcode")
			return
		}
	}
}

func ftoi(f float32) int64 {
	switch {
	case f != f:
		return 0
	case f >= 9.223372036854776e18:
		return math.MaxInt64
	case f <= -9.223372036854776e18:
		return math.MinInt64
	}
	return int64(f)
}
