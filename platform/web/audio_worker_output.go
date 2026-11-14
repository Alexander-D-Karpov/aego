//go:build js && wasm

package web

import (
	"math"
	"syscall/js"

	"aego/platform"
)

const sharedWorkerMinimumBlocks = 6

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

// workerTargetBlocks gives the shared Worker additional scheduling headroom
// without changing the local pump or the message transport. At 48 kHz with
// 512-frame blocks, the minimum reserve adds 32 ms of queued audio compared
// with the three-block shared minimum; device latency is additional.
func (a *Audio) workerTargetBlocks() int {
	target := a.TargetBlocks()
	if a.shared && a.spec.Frames > 0 {
		target = max(target, min(sharedWorkerMinimumBlocks, ringFrames/a.spec.Frames))
	}
	return target
}

// WorkerOutput returns a claimed output only after the real worklet is ready.
// data and index are undefined for the message transport and are the original
// SharedArrayBuffers for the shared transport. This does not attach a Worker.
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
	if !a.SetWorkerGate(false) {
		return false
	}
	a.workerState = nil
	a.workerReserved = false
	return true
}

// WorkerSilent describes the requested lifecycle, including a pending context
// suspension. A transition owner may keep the gain gated beyond this becoming
// false until fresh PCM has been primed and acknowledged by the worklet.
func (a *Audio) WorkerSilent() bool {
	return a == nil || a.muted || a.hidden || a.paused || !a.Running()
}

// WorkerError exposes asynchronous worklet and ownership failures to the
// backend. It does not authorize replay through the local mixer.
func (a *Audio) WorkerError() string {
	if a == nil {
		return "the audio output is missing"
	}
	return a.failed
}

// WorkerQuiet matches the final page-side report to the exact quiesce request;
// an earlier quiet report cannot authorize a later output replacement.
func (a *Audio) WorkerQuiet(token uint64) bool {
	return a != nil && !a.closed && a.workerQuiet && a.quietToken == token
}

func (a *Audio) audioCounters() platform.AudioCounters {
	return platform.AudioCounters{Underruns: a.prior.Underruns + a.underruns,
		Consumed: a.prior.Consumed + a.count.consumed, Sum: a.prior.Sum + a.sum,
		Samples: a.prior.Samples + a.samples, Suppressed: a.prior.Suppressed + a.suppressed}
}

// WorkerCounters captures the latest page-side worklet report, including any
// previously inherited device counters. Require WorkerQuiet before rebind.
func (a *Audio) WorkerCounters() platform.AudioCounters { return a.audioCounters() }

// SetWorkerBaseline preserves old output measurements on an untouched
// replacement. It must be called before attaching the replacement Worker.
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

// SelectWorkerOutput makes a reserved output the page's published mute and
// statistics target. During reopen the old output can remain selected until
// the replacement is ready, without changing the user's mute preference.
func (a *Audio) SelectWorkerOutput() bool {
	if a == nil || a.closed || !a.workerReserved {
		return false
	}
	if currentAudio != a {
		a.publish()
	}
	return true
}

// FailWorkerOutput stops audible output and publishes an attached Worker's
// failure through the existing audio statistics. It cannot restore ownership.
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

// SetWorkerStateListener observes output readiness, mute, visibility, pause
// and context-state transitions. Calls run on the page thread. It is valid only
// for a reserved output and is detached on cancellation or close.
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

// SetWorkerGate controls a separate immediate output gate. In particular,
// unmuting must not make stale PCM audible during quiesce or re-prime. It does
// not change the user's mute preference or worklet underrun counters.
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
