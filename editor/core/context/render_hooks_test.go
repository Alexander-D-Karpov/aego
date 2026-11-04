package context

import (
	"slices"
	"testing"
)

func TestResourcePreviewHooksFollowEveryOrdinaryView(t *testing.T) {
	ed := &Editor{}
	var order []string
	// The Inspector can be visited before the Scene View during UI layout.
	ed.OnRenderAfterViews(func() { order = append(order, "preview") })
	ed.OnRender(func() {
		order = append(order, "scene")
		ed.OnRenderAfterViews(func() { order = append(order, "late consumer") })
	})
	ed.OnRender(func() { order = append(order, "game") })
	ed.RunRenderHooks()
	if !slices.Equal(order, []string{"scene", "game", "preview", "late consumer"}) {
		t.Fatalf("resource consumers ran before their producers: %v", order)
	}
}

func TestRenderHooksReleaseCapturesAndRunOnceDespiteFailure(t *testing.T) {
	ed := &Editor{}
	n := 0
	ed.OnRender(func() { panic("broken view") })
	ed.OnRenderAfterViews(func() {
		n++
		ed.OnRenderAfterViews(func() { n++ })
	})
	ed.RunRenderHooks()
	if n != 1 || len(ed.notices) != 1 {
		t.Fatal("panic skipped later consumers or next-frame work ran early")
	}
	ed.RunRenderHooks()
	if n != 2 {
		t.Fatal("queued next-frame consumer was lost")
	}
	ed.RunRenderHooks()
	if n != 2 || len(ed.hooks) != 0 || len(ed.afterViewHooks) != 0 {
		t.Fatal("completed hooks ran more than once")
	}
	for _, queue := range [][]func(){ed.hooks, ed.afterViewHooks} {
		for _, fn := range queue[:cap(queue)] {
			if fn != nil {
				t.Fatal("completed hook retained a document/resource capture")
			}
		}
	}
}
