package audio

import (
	"aego/audio/pump"
	"aego/platform"
)

// A backend, when selected, exclusively owns mixer consumption and output
// production. Its failures must not replay uncertain commands through pump.
type workerBackend interface {
	Tick()
	Reopen(platform.AudioSpec) error
	Stats() pump.Stats
	Close()
}
