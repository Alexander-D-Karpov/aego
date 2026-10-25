//go:build unix

package integration

import (
	"syscall"
	"time"
)

// cpuTime is the user and system time of the process, every thread included.
func cpuTime() time.Duration {
	var ru syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &ru) != nil {
		return 0
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}
