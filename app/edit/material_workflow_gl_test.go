//go:build aego_gltest

package edit

import (
	"runtime"
	"testing"

	"aego/core/log"
)

// This GL-backed authoring workflow must fail, not falsely validate by skipping, when no graphics device is available.
func TestMaterialAuthoringWorkflowNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	materialAuthoringWorkflow(t, Config{Hidden: true, NoWatch: true, Logger: log.Nop()})
}
