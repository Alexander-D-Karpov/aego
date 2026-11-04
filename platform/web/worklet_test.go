package web

import (
	"os/exec"
	"testing"
)

// Both Web Audio transports use the same production worklet.
func TestWorkletRequestsRefillBeforePhysicalStarvation(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is unavailable")
	}
	if out, err := exec.Command(node, "testdata/refill.mjs").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
