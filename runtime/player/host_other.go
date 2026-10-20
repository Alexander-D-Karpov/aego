//go:build !js

package player

import (
	"os"
	"os/signal"
)

const hostFiles = true

func (p *Player) watchInterrupt() func() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		if _, ok := <-ch; ok {
			p.Engine.Jobs().OnMain(p.Engine.Quit)
		}
	}()
	return func() {
		signal.Stop(ch)
		close(ch)
	}
}
