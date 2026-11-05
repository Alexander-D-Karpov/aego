package editor

import "fmt"

// imageCandidate owns editor-only CPU preparation; workers never touch document buffers or GPU handles.
// The poll method publishes only current generations/revisions; replacement or closure invalidates old results.
type imageCandidate struct {
	generation   uint64
	revision     uint64
	device       uint64
	result       chan imageCandidateResult
	next         *imageCandidateRequest
	running      bool
	good         *imagePixels
	goodRevision uint64
	pending      bool
	closed       bool
	err          error
}

type imageCandidateRequest struct {
	generation, revision, device uint64
	prepare                      func() (*imagePixels, error)
}

type imageCandidateResult struct {
	generation, revision, device uint64
	image                        *imagePixels
	err                          error
}

func (c *imageCandidate) start(revision, device uint64, prepare func() (*imagePixels, error)) {
	c.generation++
	c.revision, c.device = revision, device
	c.pending, c.closed, c.err = true, false, nil
	request := &imageCandidateRequest{generation: c.generation, revision: revision, device: device, prepare: prepare}
	if c.running {
		// At most one running job and one latest immutable snapshot are held.
		// Repeated edits coalesce instead of spawning one 16 MP job per event.
		c.next = request
		return
	}
	c.launch(request)
}

func (c *imageCandidate) launch(request *imageCandidateRequest) {
	c.running = true
	out := make(chan imageCandidateResult, 1)
	c.result = out
	go func() {
		result := imageCandidateResult{generation: request.generation, revision: request.revision, device: request.device}
		defer func() {
			if panicValue := recover(); panicValue != nil {
				result.err = fmt.Errorf("image candidate importer panicked: %v", panicValue)
			}
			out <- result
		}()
		result.image, result.err = request.prepare()
	}()
}

func (c *imageCandidate) poll(revision, device uint64, lost bool) bool {
	if c.closed || c.result == nil {
		return false
	}
	select {
	case result := <-c.result:
		c.result = nil
		c.running, c.pending = false, false
		if c.next != nil {
			next := c.next
			c.next = nil
			c.pending = true
			c.launch(next)
		}
		if result.generation != c.generation || result.revision != revision || result.device != device || lost {
			return false
		}
		c.err = result.err
		if result.err == nil && result.image == nil {
			c.err = fmt.Errorf("image importer returned no candidate pixels")
		}
		if result.err != nil || result.image == nil {
			return false
		}
		c.good, c.goodRevision = result.image, result.revision
		return true
	default:
		return false
	}
}

func (c *imageCandidate) cancel() {
	c.generation++
	c.pending = false
	c.next = nil
}

func (c *imageCandidate) close() {
	c.cancel()
	c.closed = true
	c.good = nil
	c.result = nil
	c.err = nil
}
