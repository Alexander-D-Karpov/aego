package scripteditor

import (
	"testing"

	"aego/script/langsvc"
	"aego/script/workspace"
)

func TestRefreshingIncludesQueuedMainCompletion(t *testing.T) {
	ws, err := workspace.Open(workspace.Config{Root: t.TempDir(), Module: "game"})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHost(langsvc.New(ws, nil))
	var workers, main []func()
	h.d = Deps{
		Submit: func(fn func()) { workers = append(workers, fn) },
		OnMain: func(fn func()) { main = append(main, fn) },
	}
	if h.Refreshing() {
		t.Fatal("a new host must not be refreshing")
	}
	h.refresh(1)
	if !h.Refreshing() || len(workers) != 1 {
		t.Fatal("the queued language refresh was not reported")
	}
	h.refresh(2)
	if len(workers) != 1 {
		t.Fatal("a second refresh started while the first was in flight")
	}
	workers[0]()
	if !h.Refreshing() || len(main) != 1 {
		t.Fatal("refresh must remain active until its main-thread completion")
	}
	main[0]()
	if h.Refreshing() {
		t.Fatal("completed language refresh remained active")
	}
	h.refresh(3)
	if h.Refreshing() || len(workers) != 1 {
		t.Fatal("an unchanged language workspace started another refresh")
	}
}
