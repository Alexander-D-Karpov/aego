package audio

import "testing"

func TestWorldBusGainReportsTargetAndNeutralFallback(t *testing.T) {
	var absent *Service
	if absent.WorldBusGain(BusMusic) != 1 {
		t.Fatal("a missing world must have neutral gain")
	}
	s := NewService(nil, nil, nil)
	if s.WorldBusGain(BusMusic) != 1 || s.WorldBusGain(Bus(255)) != 1 {
		t.Fatal("a fresh or invalid bus must have neutral gain")
	}
	s.worldG[BusMusic] = 0.25
	if s.WorldBusGain(BusMusic) != 0.25 || s.WorldBusGain(BusSfx) != 1 {
		t.Fatal("world gain did not report the selected bus target")
	}
}
