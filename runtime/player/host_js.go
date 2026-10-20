//go:build js

package player

const hostFiles = false

func (p *Player) watchInterrupt() func() {
	return func() {}
}
