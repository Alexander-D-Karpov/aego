package editor

import (
	"errors"
	"runtime"
	"testing"
	"time"
)

func waitImageCandidate(t *testing.T, c *imageCandidate, revision, device uint64, lost bool) bool {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for c.pending && time.Now().Before(deadline) {
		if c.poll(revision, device, lost) {
			return true
		}
		runtime.Gosched()
	}
	if c.pending {
		t.Fatal("candidate worker did not finish")
	}
	return false
}

func TestImageCandidateLastGoodStaleRevisionAndClose(t *testing.T) {
	var c imageCandidate
	first, second := &imagePixels{}, &imagePixels{}
	c.start(1, 2, func() (*imagePixels, error) { return first, nil })
	if !waitImageCandidate(t, &c, 1, 2, false) || c.good != first {
		t.Fatal("valid candidate did not publish")
	}
	c.start(2, 2, func() (*imagePixels, error) { return nil, errors.New("height invalid") })
	if waitImageCandidate(t, &c, 2, 2, false) || c.good != first || c.err == nil {
		t.Fatal("failed candidate discarded last-good")
	}
	old := make(chan struct{})
	c.start(3, 2, func() (*imagePixels, error) { <-old; return second, nil })
	for range 100 {
		c.start(3, 2, func() (*imagePixels, error) { t.Error("superseded queued request ran"); return second, nil })
	}
	c.start(4, 2, func() (*imagePixels, error) { return first, nil })
	close(old)
	if !waitImageCandidate(t, &c, 4, 2, false) {
		t.Fatal("latest candidate did not publish")
	}
	if c.poll(4, 2, false) || c.good != first {
		t.Fatal("late obsolete worker replaced new candidate")
	}
	c.start(5, 2, func() (*imagePixels, error) { return second, nil })
	if waitImageCandidate(t, &c, 6, 2, false) || c.good != first {
		t.Fatal("document revision guard failed")
	}
	c.start(6, 2, func() (*imagePixels, error) { return second, nil })
	if waitImageCandidate(t, &c, 6, 3, false) || c.good != first {
		t.Fatal("old device generation published")
	}
	c.start(7, 3, func() (*imagePixels, error) { return second, nil })
	if waitImageCandidate(t, &c, 7, 3, true) || c.good != first {
		t.Fatal("lost context accepted candidate")
	}
	c.start(8, 3, func() (*imagePixels, error) { return second, nil })
	c.close()
	if c.poll(8, 3, false) || c.good != nil || !c.closed {
		t.Fatal("closed view accepted worker result")
	}
}
