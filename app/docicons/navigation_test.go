package docicons

import "testing"

func TestBuiltinReturnsIndependentPolicies(t *testing.T) {
	tasks, refs := Builtin()
	tasks["animation"] = "wrong"
	refs["aego/audio"] = "wrong"
	otherTasks, otherRefs := Builtin()
	if otherTasks["animation"] != "animation.player" || otherRefs["aego/audio"] != "cat.audio" {
		t.Fatal("one host changed the built-in policy for another")
	}
}
