package integration

import (
	"testing"

	"aego/editor/core/context"
)

type heldScriptRefresh struct {
	context.ScriptNav
	refreshing bool
}

func (h *heldScriptRefresh) Refreshing() bool { return h.refreshing }

func TestEditorSettledWaitsForScriptRefresh(t *testing.T) {
	s := workspaceSession(t)
	waitEditorSettled(t, s)
	ctx := s.App.Context()
	if ctx.Code == nil {
		t.Fatal("the editor has no script host")
	}
	original := ctx.Code
	defer func() { ctx.Code = original }()
	held := &heldScriptRefresh{ScriptNav: original, refreshing: true}
	ctx.Code = held
	if editorSettled(s) {
		t.Fatal("editor was considered settled while script analysis was still refreshing")
	}
	held.refreshing = false
	waitEditorSettled(t, s)
}

var frameAllocationSink *[128]byte

func TestEditorFrameAllocsDetectsAddedAllocations(t *testing.T) {
	skipUnderRace(t)
	s := workspaceSession(t)
	assets := s.App.AssetsPanel()
	before := assets.DirectoryRefreshes()
	baseline := editorFrameAllocs(t, s, func() { s.Step() })
	if assets.DirectoryRefreshes() <= before {
		t.Fatal("the allocation window did not wait for a real Assets refresh")
	}
	withAllocation := editorFrameAllocs(t, s, func() {
		s.Step()
		frameAllocationSink = new([128]byte)
	})
	if withAllocation <= baseline {
		t.Fatalf("an allocation on every frame went undetected: baseline=%v with allocation=%v", baseline, withAllocation)
	}
}
