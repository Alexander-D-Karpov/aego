package integration

import "testing"

// skipUnderRace skips a per-frame allocation comparison: the race detector's instrumentation moves some values to the heap, so the counts say nothing; make test-nonative runs these without it.
func skipUnderRace(t *testing.T) {
	t.Helper()
	if raceDetector {
		t.Skip("frame allocations are not comparable under the race detector")
	}
}
