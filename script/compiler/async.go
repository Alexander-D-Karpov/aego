package compiler

import (
	"aego/script/ast"
	"aego/script/bytecode"
	"aego/script/checker"
)

func funcSlots(f *checker.Func) []int {
	out := make([]int, len(f.Params))
	for i, p := range f.Params {
		out[i] = p.Type.Slots()
	}
	return out
}

func signalSlots(sg *checker.ScriptSignal) []int {
	out := make([]int, len(sg.Params))
	for i, p := range sg.Params {
		out[i] = p.Slots()
	}
	return out
}

func waitKind(k ast.YieldKind) uint8 {
	switch k {
	case ast.YieldWait:
		return bytecode.WaitTime
	case ast.YieldWaitUnscaled:
		return bytecode.WaitUnscaled
	case ast.YieldFrames:
		return bytecode.WaitFrames
	case ast.YieldFixed:
		return bytecode.WaitFixed
	case ast.YieldAwait:
		return bytecode.WaitAwait
	}
	return bytecode.WaitNext
}

func (c *fn) startTo(e *ast.StartExpr, dst int) {
	f := c.u.a.Starts[e]
	if f == nil {
		return
	}
	idx, ok := c.u.funcs[f]
	if !ok {
		return
	}
	c.invoke(bytecode.START, idx, e.Args, funcSlots(f), 1, dst, true)
}

func (c *fn) yield(s *ast.YieldStmt) {
	kind := int(waitKind(s.Kind))
	if s.Value == nil || s.Kind == ast.YieldNext {
		c.b.ABC(bytecode.YIELD, 0, 0, kind)
		return
	}
	mark := c.top
	r := c.operand(s.Value)
	c.b.ABC(bytecode.YIELD, r, 0, kind)
	c.top = mark
}

func (c *fn) selfOr(e ast.Expr, dst int) {
	if e == nil {
		c.b.ABC(bytecode.SELFREF, dst, 0, 0)
		return
	}
	c.exprTo(e, dst)
}

func (c *fn) asyncCall(e *ast.CallExpr, call *checker.Call, dst int, want bool) {
	if !want {
		dst = c.alloc(1)
	}
	switch call.Kind {
	case checker.CallEmit:
		c.invoke(bytecode.EMIT, call.Signal.Index, e.Args, signalSlots(call.Signal), 0, dst, false)
	case checker.CallConnect:
		sg, h := call.Signal, call.Handler
		idx := c.u.connect(bytecode.ConnectDef{
			Source:  sg.Script.FullName,
			Signal:  sg.Name,
			Target:  h.Script.FullName,
			Handler: h.Name,
		})
		mark := c.top
		w := c.alloc(3)
		c.selfOr(call.Recv, w+1)
		c.selfOr(call.HandlerRecv, w+2)
		c.b.ABx(bytecode.CONNECT, w, idx)
		c.move(dst, w, 1)
		c.top = mark
	case checker.CallDisconnect:
		mark := c.top
		r := c.operand(call.Recv)
		c.b.ABC(bytecode.DISCONNECT, r, 0, 0)
		c.top = mark
	case checker.CallConnected:
		mark := c.top
		r := c.operand(call.Recv)
		c.b.ABC(bytecode.CONNECTED, dst, r, 0)
		c.top = mark
	case checker.CallCoStop:
		mark := c.top
		r := c.operand(call.Recv)
		c.b.ABC(bytecode.COSTOP, r, 0, 0)
		c.top = mark
	case checker.CallCoRunning:
		mark := c.top
		r := c.operand(call.Recv)
		c.b.ABC(bytecode.CORUN, dst, r, 0)
		c.top = mark
	case checker.CallGetScript:
		c.exprTo(call.Recv, dst)
		c.b.ABx(bytecode.GETSCRIPT, dst, c.u.scriptRef(call.Target))
	case checker.CallScriptMethod:
		f := call.Func
		args := append([]ast.Expr{call.Recv}, e.Args...)
		params := append([]int{1}, funcSlots(f)...)
		c.invoke(bytecode.CALLR, c.u.xmethod(f), args, params, f.Result.Slots(), dst, want)
	}
}
