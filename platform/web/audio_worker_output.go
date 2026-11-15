//go:build js && wasm

package web

import (
	"math"
	"syscall/js"

	"aego/platform"
)

const sharedWorkerMinimumBlocks = 6

// ReserveWorkerOutput claims untouched output before PCM; keep the mixer unrendered and local pump stopped, including during worklet creation.
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

// A loading worklet reads refillThreshold on attach; MessagePort orders updates to an existing one.
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

// workerTargetBlocks adds shared-mode headroom, about 32 ms at 48 kHz/512 frames versus three blocks plus device latency, without changing local or message mode.
func (a *Audio) workerTargetBlocks() int {
	target := a.TargetBlocks()
	if a.shared && a.spec.Frames > 0 {
		target = max(target, min(sharedWorkerMinimumBlocks, ringFrames/a.spec.Frames))
	}
	return target
}

// WorkerOutput returns a ready claimed worklet without attaching a Worker; data/index are the original shared buffers or undefined in message mode.
func (a *Audio) WorkerOutput() (node, data, index js.Value, block, target int, ready bool) {
	if a == nil || a.closed || !a.workerReserved || a.workerAttached || !a.ready || a.failed != "" || !a.node.Truthy() {
		return
	}
	node, block, target = a.node, a.spec.Frames, a.workerTargetBlocks()*a.spec.Frames
	if a.shared {
		if !a.data.Truthy() || !a.index.Truthy() {
			return js.Value{}, js.Value{}, js.Value{}, 0, 0, false
		}
		data, index = a.data.Get("buffer"), a.index.Get("buffer")
	}
	return node, data, index, block, target, true
}

// CommitWorkerOutput records certain or uncertain attachment, including reported failures, and permanently forbids local-pump fallback.
func (a *Audio) CommitWorkerOutput() bool {
	if a == nil || !a.workerReserved || a.workerAttached {
		return false
	}
	a.workerAttached = true
	return true
}

// CancelWorkerOutput restores only an uncommitted output after Worker termination and confirmed CancelUnsent, never an uncertain attachment.
func (a *Audio) CancelWorkerOutput() bool {
	if a == nil || a.closed || !a.workerReserved || a.workerAttached {
		return false
	}
	if !a.workerRefillThreshold(0) {
		a.failed = "the worker refill threshold could not be cleared"
		return false
	}
	if !a.SetWorkerGate(false) {
		return false
	}
	a.workerState = nil
	a.workerReserved = false
	return true
}

// WorkerSilent includes pending suspension; keep gain gated after it clears until fresh PCM is primed and acknowledged.
func (a *Audio) WorkerSilent() bool {
	return a == nil || a.muted || a.hidden || a.paused || !a.Running()
}

// WorkerError exposes asynchronous worklet and ownership failures without authorizing local replay.
func (a *Audio) WorkerError() string {
	if a == nil {
		return "the audio output is missing"
	}
	return a.failed
}

// WorkerQuiet matches the final report to this quiesce request, not an earlier one.
func (a *Audio) WorkerQuiet(token uint64) bool {
	return a != nil && !a.closed && a.workerQuiet && a.quietToken == token
}

func (a *Audio) audioCounters() platform.AudioCounters {
	return platform.AudioCounters{Underruns: a.prior.Underruns + a.underruns,
		Consumed: a.prior.Consumed + a.count.consumed, Sum: a.prior.Sum + a.sum,
		Samples: a.prior.Samples + a.samples, Suppressed: a.prior.Suppressed + a.suppressed}
}

// WorkerCounters includes inherited device counters; require WorkerQuiet before rebind.
func (a *Audio) WorkerCounters() platform.AudioCounters { return a.audioCounters() }

// SetWorkerBaseline preserves measurements on an untouched replacement before Worker attachment.
func (a *Audio) SetWorkerBaseline(c platform.AudioCounters) bool {
	if a == nil || a.closed || !a.workerReserved || a.workerAttached || a.sentAudio || a.underruns != 0 ||
		a.count.consumed != 0 || a.sum != 0 || a.samples != 0 || a.suppressed != 0 ||
		c.Underruns < 0 || c.Consumed < 0 || c.Suppressed < 0 || c.Sum < 0 || c.Samples < 0 ||
		math.IsNaN(c.Sum) || math.IsInf(c.Sum, 0) || math.IsNaN(c.Samples) || math.IsInf(c.Samples, 0) {
		return false
	}
	a.prior = c
	return true
}

// SelectWorkerOutput selects the reserved mute/statistics target; reopening may retain the old output without changing user mute.
func (a *Audio) SelectWorkerOutput() bool {
	if a == nil || a.closed || !a.workerReserved {
		return false
	}
	if currentAudio != a {
		a.publish()
	}
	return true
}

// FailWorkerOutput silences and reports an attached Worker's failure without restoring ownership.
func (a *Audio) FailWorkerOutput(message string) {
	if a == nil || a.closed || !a.workerReserved {
		return
	}
	if !a.SetWorkerGate(true) {
		return
	}
	if message == "" {
		message = "the audio worker failed"
	}
	a.failed = message
}

// SetWorkerStateListener observes readiness, mute, visibility, pause and context state on the page thread; it requires a reservation and detaches on close or cancellation.
func (a *Audio) SetWorkerStateListener(fn func()) bool {
	if a == nil || a.closed || !a.workerReserved {
		return false
	}
	a.workerState = fn
	return true
}

func (a *Audio) notifyWorkerState() {
	if a.workerState != nil && !a.closed {
		a.workerState()
	}
}

// SetWorkerGate controls the immediate output gate against stale PCM without changing user mute or underrun counters.
func (a *Audio) SetWorkerGate(on bool) (ok bool) {
	if a == nil || a.closed || !a.workerReserved {
		return false
	}
	defer func() {
		if recover() != nil {
			a.failed = "the worker output gate could not be changed"
			func() {
				defer func() { _ = recover() }()
				if a.ctx.Truthy() {
					a.quiet(try(a.ctx, "suspend"))
				}
			}()
			ok = false
		}
	}()
	a.workerGate = on
	if a.gain.Truthy() {
		param := a.gain.Get("gain")
		now := a.ctx.Get("currentTime")
		param.Call("cancelScheduledValues", now)
		param.Call("setValueAtTime", a.level(), now)
	}
	return true
}
