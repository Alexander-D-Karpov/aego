//go:build js

package pipeline

import (
	"errors"
	"os"
)

// Browser asset storage has its own transaction. A browser cannot hold a host
// index lock or authorize disk source publication and recovery.
func lockIndexGuard(*os.File) error {
	return errors.New("host source transactions are unavailable in a browser")
}

func unlockIndexGuard(*os.File) {}
func processAlive(int) bool     { return true }
