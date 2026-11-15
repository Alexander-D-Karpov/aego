//go:build !aego_runtime

package docs

import "testing"

func TestSummaryNeverUsesCodeAsDescription(t *testing.T) {
	for _, code := range []string{"p, _ := sdk.Get[sdk.AnimationPlayer](node)", "type Fader struct {", "var attack timer.Timer"} {
		for _, indent := range []string{"    ", "\t"} {
			md := "# Go\n\n" + indent + code + "\n\nUse this API from a native module."
			if got := summary(md); got != "Use this API from a native module." {
				t.Errorf("summary(%q) = %q", md, got)
			}
		}
	}
	if got := summary("# Example\n\n```go\ntype Fader struct {}\n```\n\n    var attack timer.Timer"); got != "" {
		t.Errorf("code-only article acquired a description: %q", got)
	}
}
