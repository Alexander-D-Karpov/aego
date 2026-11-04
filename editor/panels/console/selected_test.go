package console

import (
	"aego/core/log"
	"strings"
	"testing"
)

func TestSelectedRenderEntrySurvivesClearAndRingWrap(t *testing.T) {
	s := NewSink(16)
	lg := log.New(s)
	lg.Log(log.Render, log.Error, "shader candidate failed", log.F("code", "SHD009"), log.F("file", "assets/postfx/cold"), log.F("line", 0), log.F("err", "GL33 link/layout: MAT004: postfx shader cannot draw sprite geometry"), log.F("asset", "45e397813812442d994a810000000001"))
	selected := s.Snapshot(nil)[0]
	m := selected.Message()
	if m.Level != "ERROR" || m.Channel != "render" || m.Code != "SHD009" || m.Cause != "MAT004" || m.Path != "assets/postfx/cold" || m.Line != 0 || m.Asset == "" {
		t.Fatalf("%+v", m)
	}
	p := New(s)
	p.refresh()
	p.selected, p.selectedID = 0, selected.ID
	for n := 0; n < 32; n++ {
		lg.Log(log.Game, log.Info, "another entry", log.F("n", n))
	}
	p.refresh()
	if p.selected != -1 {
		t.Fatal("selection moved to another entry")
	}
	s.Clear()
	if !strings.Contains(selected.details(), "MAT004") || len(s.Snapshot(nil)) != 0 {
		t.Fatal("lost selected snapshot after Clear")
	}
	for _, x := range s.ring {
		if x.Text != "" {
			t.Fatal("Clear retained text references")
		}
	}
}
