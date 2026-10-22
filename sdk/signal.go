package sdk

import (
	"errors"
	"strings"

	"aego/contracts/signals"
	"aego/engine"
	"aego/engine/module"
	"aego/schema"
	"aego/script/bind"
	"aego/script/vm"
)

type (
	Signal0              = module.Signal0
	Signal1[A any]       = module.Signal1[A]
	Signal2[A, B any]    = module.Signal2[A, B]
	Signal3[A, B, C any] = module.Signal3[A, B, C]
	SignalHandler        = signals.Handler
	Connection           = vm.Value
)

var (
	errNoHub    = errors.New("signals need the script runtime in this world")
	errNoSignal = errors.New("no such signal on the node")
)

func hubOf(ctx *Context) (signals.Hub, bool) {
	if ctx == nil || ctx.World == nil {
		return nil, false
	}
	return engine.ServiceOf[signals.Hub](ctx.World, signals.ServiceID)
}

func componentSignal(reg *schema.Registry, comp, name string) (*schema.TypeDescriptor, int) {
	td, ok := reg.ByName(comp)
	if !ok {
		if td, ok = reg.ComponentNamed(comp); !ok {
			return nil, -1
		}
	}
	for i, s := range td.Signals {
		if s.Name == name {
			return td, i
		}
	}
	return nil, -1
}

func Connect(ctx *Context, node NodeHandle, path string, fn SignalHandler) (Connection, error) {
	hub, ok := hubOf(ctx)
	if !ok {
		return Connection{}, errNoHub
	}
	i := strings.LastIndexByte(path, '.')
	if i <= 0 {
		return Connection{}, errNoSignal
	}
	owner, name := path[:i], path[i+1:]
	var c Connection
	if script, ok := strings.CutPrefix(owner, "script/"); ok {
		c = hub.ConnectScript(node, script, name, fn)
	} else if s := SceneOf(ctx); s != nil {
		if td, idx := componentSignal(s.Registry(), owner, name); td != nil {
			src := signals.Source{Kind: signals.KindComponent, A: bind.NodeValue(node).A, B: uint64(td.ID)}
			c = hub.Connect(src, idx, fn)
		}
	}
	if c.IsZero() {
		return c, errNoSignal
	}
	return c, nil
}

func Connect0(ctx *Context, node NodeHandle, path string, fn func()) (Connection, error) {
	return Connect(ctx, node, path, func([]vm.Value) { fn() })
}

func Connect1[A any](ctx *Context, node NodeHandle, path string, fn func(A)) (Connection, error) {
	enc, err := bind.NewEncoder[A](bind.New())
	if err != nil {
		return Connection{}, err
	}
	return Connect(ctx, node, path, func(args []vm.Value) { fn(enc.Get(args)) })
}

func Disconnect(ctx *Context, c Connection) {
	if hub, ok := hubOf(ctx); ok {
		hub.Disconnect(c)
	}
}
