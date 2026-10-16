//go:build aego_runtime

package scripting_test

import (
	"strings"
	"testing"

	"aego/script/bind"
)

func TestReflectionFallbackFailsRuntimeBuild(t *testing.T) {
	err := build(func(*bind.Ctx, *Probe) {})
	if err == nil || !strings.Contains(err.Error(), "needs a typed adapter") {
		t.Fatalf("err = %v", err)
	}
}
