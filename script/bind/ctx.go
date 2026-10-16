package bind

import (
	"errors"
	"fmt"

	"aego/core/log"
	"aego/core/rng"
	"aego/engine"
	"aego/platform/input"
	"aego/scene"
	"aego/script/vm"
)

var errNoCtx = errors.New("host function called without a binding context")

type Ctx struct {
	World   *engine.World
	Scene   *scene.Scene
	Input   *input.State
	Random  *rng.Source
	Log     *log.ChannelLogger
	Node    scene.NodeHandle
	Recv    scene.NodeHandle
	Self    vm.InstanceID
	Dt      float32
	Fixed   float32
	Elapsed float64
	Frame   uint64
	err     error
}

func (c *Ctx) Fail(msg string) {
	if c.err == nil {
		c.err = errors.New(msg)
	}
}

func (c *Ctx) Failf(format string, args ...any) {
	if c.err == nil {
		c.err = fmt.Errorf(format, args...)
	}
}

func (c *Ctx) Failed() bool {
	return c.err != nil
}

func (c *Ctx) finish() error {
	err := c.err
	c.err = nil
	return err
}

func ctxOf(m *vm.Machine) (*Ctx, error) {
	c, ok := m.Env.(*Ctx)
	if !ok || c == nil {
		return nil, errNoCtx
	}
	return c, nil
}
