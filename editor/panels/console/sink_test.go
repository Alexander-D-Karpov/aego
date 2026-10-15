package console

import (
	"testing"

	"aego/core/log"
)

func TestCollapseAndRing(t *testing.T) {
	s := NewSink(16)
	lg := log.New(s)
	for i := 0; i < 3; i++ {
		lg.Log(log.Game, log.Warn, "same")
	}
	lg.Log(log.Game, log.Error, "other", log.F("k", 1))
	got := s.Snapshot(nil)
	if len(got) != 2 || got[0].Count != 3 || got[1].Text != "other k=1" {
		t.Fatalf("entries = %+v", got)
	}
	for i := 0; i < 40; i++ {
		lg.Log(log.Game, log.Info, "line", log.F("i", i))
	}
	got = s.Snapshot(nil)
	if len(got) != 16 || got[15].Text != "line i=39" {
		t.Fatalf("ring kept %d, last %q", len(got), got[len(got)-1].Text)
	}
	info, warn, errs := s.Counts()
	if info != 40 || warn != 3 || errs != 1 {
		t.Fatalf("counts = %d %d %d", info, warn, errs)
	}
	s.Clear()
	if len(s.Snapshot(nil)) != 0 {
		t.Fatal("clear failed")
	}
}
