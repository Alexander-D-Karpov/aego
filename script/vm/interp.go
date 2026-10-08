package vm

import (
	"math"
	"strconv"

	"aego/script/bytecode"
)

func (m *Machine) run() {
	top := len(m.frames) - 1
	fr := m.frames[top]
	f, base, pc := fr.f, fr.base, fr.pc
	code := f.code
	regs := m.stack[base : base+f.Regs]
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
			if len(m.frames) >= m.opt.MaxCallDepth {
				m.trap(top, pc, CodeCallDepth, "call depth exceeds "+strconv.Itoa(m.opt.MaxCallDepth))
				return
			}
			nb := base + a
			if need := nb + callee.Regs; need > len(m.stack) {
				m.grow(need)
			}
			clear(m.stack[nb+callee.Params : nb+callee.Regs])
			m.frames[top].pc = pc
			m.frames = append(m.frames, frame{f: callee, base: nb})
			top++
			m.stats.Calls++
			f, base, pc, code = callee, nb, 0, callee.code
			regs = m.stack[base : base+f.Regs]
		case bytecode.CALLH:
			bx := int(in.Bx())
			m.frames[top].pc = pc
			m.stats.HostCalls++
			if err := m.host(f.mod.hosts[bx], regs[a:a+f.mod.windows[bx]]); err != nil {
				if m.fault == nil {
					m.raise(CodeHost, err.Error())
				}
				return
			}
		case bytecode.RET:
			copy(m.stack[base:base+b], regs[a:a+b])
			m.frames = m.frames[:top]
			if top == 0 {
				return
			}
			top--
			fr := &m.frames[top]
			f, base, pc, code = fr.f, fr.base, fr.pc, fr.f.code
			regs = m.stack[base : base+f.Regs]
		case bytecode.GETSF, bytecode.SETSF:
			bx := int(in.Bx())
			if bx >= len(m.self) {
				m.trap(top, pc, CodeLink, "script field is outside the instance")
				return
			}
			if in.Op() == bytecode.GETSF {
				regs[a] = m.self[bx]
			} else {
				m.self[bx] = regs[a]
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

// ftoi converts float32 to int64 deterministically across architectures. Go's
// int64(float32) is platform-defined for NaN and out-of-range inputs (amd64
// gives MinInt64, arm64 saturates); pinning it keeps replay reproducible.
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
