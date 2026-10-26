package agent

import (
	"strconv"
	"strings"
	"testing"
)

func tree(depth, fan int) NodeView {
	v := NodeView{Name: "n"}
	if depth > 0 {
		for i := 0; i < fan; i++ {
			v.Children = append(v.Children, tree(depth-1, fan))
		}
	}
	return v
}

func TestCapTreeKeepsTheFirstNodes(t *testing.T) {
	v := tree(4, 4)
	all := nodeCount(&v)
	left := capTree(&v, 50, 0)
	if kept := nodeCount(&v); kept != 50 || kept+left != all {
		t.Fatalf("kept %d and left %d of %d", kept, left, all)
	}
	more := 0
	var walk func(n *NodeView)
	walk = func(n *NodeView) {
		more += n.More
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	walk(&v)
	if more != left {
		t.Fatalf("the more counts add up to %d, %d were left out", more, left)
	}
	v = tree(4, 2)
	if left := capTree(&v, 0, 2); nodeCount(&v) != 7 || left != 31-7 {
		t.Fatalf("depth 2 kept %d nodes", nodeCount(&v))
	}
}

func TestScriptWindow(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 1000; i++ {
		b.WriteString("line " + strconv.Itoa(i) + "\n")
	}
	text := b.String()
	if got, note := window(text, 0, 0, 2000); got != text || note != "" {
		t.Fatal("a short script is not read whole")
	}
	got, note := window(text, 0, 0, 400)
	if !strings.HasPrefix(got, "line 1\n") || !strings.HasSuffix(got, "line 400\n") || !strings.Contains(note, "1-400 of 1001") {
		t.Fatalf("the first window is wrong: %q", note)
	}
	got, _ = window(text, 600, 620, 400)
	if !strings.HasPrefix(got, "line 600\n") || !strings.HasSuffix(got, "line 620\n") {
		t.Fatalf("the asked range is wrong: %q", got[:20])
	}
}
