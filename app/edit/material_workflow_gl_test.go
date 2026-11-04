//go:build aego_gltest

package edit

import (
	"runtime"
	"testing"

	"aego/core/log"
)

// The same authoring workflow also runs through the real GL-backed editor.
// An unavailable graphics device is a failure for this explicit integration
// target, rather than a skipped test reported as a successful GPU validation.
func TestMaterialAuthoringWorkflowNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	materialAuthoringWorkflow(t, Config{Hidden: true, NoWatch: true, Logger: log.Nop()})
}
