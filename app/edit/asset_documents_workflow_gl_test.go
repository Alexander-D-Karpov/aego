//go:build aego_gltest

package edit

import (
	"runtime"
	"testing"

	"aego/core/log"
)

func TestImageAssetAuthoringWorkflowNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	imageAssetAuthoringWorkflow(t, Config{Hidden: true, NoWatch: true, Width: 1600, Height: 1000, Logger: log.Nop()})
}
