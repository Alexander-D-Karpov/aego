//go:build !windows

package pump

import "time"

type mixClockSample struct{ start time.Time }

func mixClockStart() mixClockSample                  { return mixClockSample{start: time.Now()} }
func mixClockElapsed(s mixClockSample) time.Duration { return time.Since(s.start) }
