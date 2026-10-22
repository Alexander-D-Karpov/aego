package integration

import (
	"testing"

	"aego/contracts/debugdraw"
	"aego/editor/core/context"
	"aego/editor/shell"
	"aego/engine"
	"aego/modules/physics"
	pheditor "aego/modules/physics/editor"
)

// The physics debug channels follow the Game View overlays even in full-screen Play, where no Scene View draws; turning the overlays off hides them without touching the layer flags.
func TestPhysicsLayersFollowGameOverlays(t *testing.T) {
	s, pc := rewindSession(t)
	if err := s.App.Context().Commands.Run(shell.CommandFullscreenPlay); err != nil {
		t.Fatal(err)
	}
	o := &s.App.Context().Overlay
	o.Set(pheditor.CmdDebug, true)
	o.Set(pheditor.FlagShapes, true)
	o.Set(pheditor.FlagJoints, true)
	d, ok := engine.ServiceOf[debugdraw.Draw](pc.World(), debugdraw.ServiceID)
	if !ok {
		t.Fatal("the play world has no debug draw")
	}
	visible := func() (shapes, joints bool) {
		s.Step()
		return d.Wants(physics.ChannelShapes), d.Wants(physics.ChannelJoints)
	}
	o.Set(context.OverlayGame, true)
	if sh, j := visible(); !sh || !j {
		t.Fatalf("overlays on: shapes %v joints %v, want both", sh, j)
	}
	o.Set(context.OverlayGame, false)
	if sh, j := visible(); sh || j {
		t.Fatalf("overlays off: shapes %v joints %v, want none", sh, j)
	}
	if !o.On(pheditor.FlagShapes) || !o.On(pheditor.FlagJoints) {
		t.Fatal("turning the overlays off changed the layer flags")
	}
	o.Set(context.OverlayGame, true)
	if sh, j := visible(); !sh || !j {
		t.Fatalf("overlays back on: shapes %v joints %v, want both again", sh, j)
	}
}
