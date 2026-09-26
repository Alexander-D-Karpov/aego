package headless

import (
	"aego/demos"
	"aego/demos/basic"
	"aego/engine"
)

func init() {
	demos.Register("headless", func() demos.Demo { return New() })
}

type Demo struct {
	*basic.Demo
}

func New() *Demo {
	return &Demo{Demo: basic.New()}
}

func (d *Demo) Name() string {
	return "headless"
}

func (d *Demo) Configure(c *engine.Config) {
	c.Headless = true
	if c.HeadlessFrameTime <= 0 {
		c.HeadlessFrameTime = 1.0 / 60
	}
}
