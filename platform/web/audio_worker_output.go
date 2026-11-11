//go:build js && wasm

package web

import "syscall/js"

// ReserveWorkerOutput claims an untouched Web Audio output before either the
// local pump or a Worker publishes PCM. The owner must also keep the mixer
// unrendered and must not start the local pump while this claim is held.
// Claiming can precede asynchronous AudioWorklet creation.
func (a *Audio) ReserveWorkerOutput() bool {
	if a == nil || a.closed || a.failed != "" || a.workerReserved || a.sentAudio || a.priming || a.refill != nil {
		return false
	}
	a.workerReserved = true
	if !a.workerRefillThreshold(a.refillThreshold()) {
		a.workerReserved = false
		a.failed = "the worker refill threshold could not be configured"
		return false
	}
	return true
}

// The worklet may already exist when claimed. A loading worklet instead reads
// refillThreshold during attach. MessagePort preserves these updates in order.
func (a *Audio) workerRefillThreshold(frames int) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	if a.port.Truthy() {
		a.port.Call("postMessage", map[string]any{"refillAt": frames})
	}
	return true
}

// WorkerOutput returns a claimed output only after the real worklet is ready.
// data and index are undefined for the message transport and are the original
// SharedArrayBuffers for the shared transport. This does not attach a Worker.
func (a *Audio) WorkerOutput() (node, data, index js.Value, block, target int, ready bool) {
	if a == nil || a.closed || !a.workerReserved || a.workerAttached || !a.ready || a.failed != "" || !a.node.Truthy() {
		return
	}
	node, block, target = a.node, a.spec.Frames, a.TargetBlocks()*a.spec.Frames
	if a.shared {
		if !a.data.Truthy() || !a.index.Truthy() {
			return js.Value{}, js.Value{}, js.Value{}, 0, 0, false
		}
		data, index = a.data.Get("buffer"), a.index.Get("buffer")
	}
	return node, data, index, block, target, true
}

// CommitWorkerOutput permanently records an attachment or an uncertain
// attachment attempt. Call it for both readiness and failures reported as
// attached by the host; that output must not return to the local pump.
func (a *Audio) CommitWorkerOutput() bool {
	if a == nil || !a.workerReserved || a.workerAttached {
		return false
	}
	a.workerAttached = true
	return true
}

// CancelWorkerOutput returns an uncommitted output to the local pump only
// after its owner has terminated the Worker and confirmed CancelUnsent. It is
// not crash recovery and cannot undo a committed or uncertain attachment.
func (a *Audio) CancelWorkerOutput() bool {
	if a == nil || a.closed || !a.workerReserved || a.workerAttached {
		return false
	}
	if !a.workerRefillThreshold(0) {
		a.failed = "the worker refill threshold could not be cleared"
		return false
	}
	a.workerReserved = false
	return true
}
