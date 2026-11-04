package problems

import (
	"testing"

	"aego/editor/core/context"
)

func TestWhereOmitsUnknownSourcePositions(t *testing.T) {
	for _, tt := range []struct {
		line, col int
		want      string
	}{
		{10, 0, "10"},
		{10, -1, "10"},
		{10, 4, "10:4"},
		{0, 0, ""},
		{0, 4, ""},
	} {
		if got := where(&context.ScriptProblem{Line: tt.line, Col: tt.col}); got != tt.want {
			t.Errorf("where(%d, %d) = %q, want %q", tt.line, tt.col, got, tt.want)
		}
	}
}
