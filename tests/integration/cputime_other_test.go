//go:build !unix

package integration

import "time"

func cpuTime() time.Duration { return 0 }
