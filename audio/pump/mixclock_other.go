//go:build !windows

package pump

import "time"

type mixTimer struct{ start time.Time }

func startMixTimer() mixTimer { return mixTimer{start: time.Now()} }

func (t mixTimer) elapsed() time.Duration { return time.Since(t.start) }
