//go:build js && wasm

package audio

import (
	"errors"
	"fmt"
	"syscall/js"
	"time"

	"aego/audio/mix"
	"aego/audio/pump"
	"aego/audio/workerproto"
	"aego/core/log"
	"aego/platform"
)

// The platform supplies only browser objects and device lifecycle. This keeps
// platform/web independent of the mixer and its Worker transport.
type browserWorkerOutput interface {
	platform.AudioOutput
	ReserveWorkerOutput() bool
	WorkerOutput() (node, data, index js.Value, block, target int, ready bool)
	CommitWorkerOutput() bool
	CancelWorkerOutput() bool
	WorkerSilent() bool
	WorkerError() string
	WorkerQuiet(uint64) bool
	WorkerCounters() platform.AudioCounters
	SetWorkerBaseline(platform.AudioCounters) bool
	SelectWorkerOutput() bool
	FailWorkerOutput(string)
	SetWorkerStateListener(func()) bool
	SetWorkerGate(bool) bool
}

type browserWorkerPhase uint8

const (
	workerStarting browserWorkerPhase = iota + 1
	workerActive
	workerQuiescing
	workerQuiet
	workerResuming
	workerRebinding
	workerFailed
	workerClosed
)

const (
	workerStartupTimeout = 10 * time.Second
	workerMemoryBudget   = 512 << 20
)

var errWorkerReopen = errors.New("audio worker: output reopen is unavailable")

type pendingWorkerOutput struct {
	out       browserWorkerOutput
	want      platform.AudioSpec
	deadline  *time.Timer
	waitQuiet bool
	cut       bool
}

type browserWorker struct {
	m            *Module
	out          browserWorkerOutput
	reactor      js.Value
	url          string
	createWorker func(string) (js.Value, error)
	openOutput   func(platform.AudioSpec) platform.AudioOutput
	worker       js.Value
	browser      *workerproto.Browser
	phase        browserWorkerPhase
	clock        workerQuietClock
	now          func() time.Time
	deadline     *time.Timer
	driving      bool
	constructing bool
	readyPending bool
	requiesce    bool
	failure      error
	attached     bool
	gated        bool
	stats        workerproto.BrowserStats
	hasStats     bool
	pending      *pendingWorkerOutput
}

// newBrowserWorker is constructed before the local pump starts, with a
// compiled reactor and dedicated Worker.
func newBrowserWorker(m *Module, out browserWorkerOutput, reactor js.Value, url string,
	create func(string) (js.Value, error)) (*browserWorker, error) {
	if m == nil || m.mixer == nil || out == nil || !reactor.Truthy() || url == "" || create == nil {
		return nil, errors.New("audio worker: invalid startup options")
	}
	if !out.ReserveWorkerOutput() {
		return nil, nil
	}
	r := &browserWorker{m: m, out: out, reactor: reactor, url: url, createWorker: create, openOutput: m.open,
		phase: workerStarting, now: time.Now}
	if !out.SetWorkerGate(true) || !out.SetWorkerStateListener(r.outputChanged) {
		if !out.CancelWorkerOutput() {
			return nil, errors.New("audio worker: could not release failed startup output")
		}
		return nil, nil
	}
	r.gated = true
	r.deadline = time.AfterFunc(workerStartupTimeout, func() {
		if r.phase == workerStarting {
			r.fail(errors.New("audio worker: startup timed out"), false)
		}
	})
	r.Tick()
	if r.phase == workerClosed {
		return nil, nil
	}
	if r.phase == workerFailed {
		return nil, r.failure
	}
	return r, nil
}

func (r *browserWorker) outputChanged() {
	if r.phase == workerClosed || r.phase == workerFailed {
		return
	}
	// Mute/hidden/pause must suppress output before the platform can make a
	// later resume audible. Tick performs the asynchronous flush afterwards.
	if r.out.WorkerSilent() && !r.gate(true) {
		return
	}
	r.Tick()
}

func (r *browserWorker) gate(on bool) bool {
	if r.gated == on {
		return true
	}
	if !r.out.SetWorkerGate(on) {
		r.fail(errors.New("audio worker: output gate failed"), r.attached)
		return false
	}
	r.gated = on
	return true
}

func (r *browserWorker) Tick() {
	if r == nil || r.driving || r.phase == workerClosed || r.phase == workerFailed {
		return
	}
	r.driving = true
	defer func() { r.driving = false }()
	if r.phase == workerRebinding {
		return
	}
	if message := r.out.WorkerError(); message != "" {
		r.fail(errors.New(message), r.attached)
		return
	}
	if r.phase == workerStarting {
		r.startWhenReady()
		return
	}
	// Once the last old-rate skip is queued, leave the clock at that exact
	// boundary until the quiet replacement adopts the new sample rate.
	frozen := r.pending != nil && r.pending.cut && r.clock.started && r.out.WorkerSilent()
	if !frozen && !r.reconcile(r.now()) {
		return
	}
	if err := r.browser.PollStreams(); err != nil {
		r.fail(err, true)
		return
	}
	if !r.tryRebind() {
		r.tryResume()
	}
}

func (r *browserWorker) startWhenReady() {
	if r.browser != nil || r.constructing {
		return
	}
	node, data, index, block, target, ready := r.out.WorkerOutput()
	if !ready {
		return
	}
	worker, err := r.createWorker(r.url)
	if err != nil {
		r.fail(err, false)
		return
	}
	r.worker, r.constructing = worker, true
	spec := r.out.Spec()
	b, err := workerproto.NewBrowser(r.m.mixer, workerproto.BrowserOptions{
		Worker: worker, Module: r.reactor, Node: node, Data: data, Index: index,
		Block: block, Target: target, RequireStats: true,
		Init: workerproto.Request{Rate: spec.SampleRate, Frames: block, Voices: Voices,
			Buses: BusDescs(), Budget: workerMemoryBudget},
		OnReady: func() {
			if r.constructing {
				r.readyPending = true
				return
			}
			r.outputReady()
		},
		OnQuiesced: func() {
			if r.phase == workerQuiescing {
				r.phase = workerQuiet
				if !r.tryRebind() {
					r.tryResume()
				}
			}
		},
		OnRebind: func() { r.outputRebound() },
		OnResume: func() { r.outputResumed() },
		OnBatch: func() {
			if r.browser != nil {
				if stats, ok := r.browser.Stats(); ok {
					r.stats, r.hasStats = stats, true
				}
			}
		},
		OnIdle: func() {
			if !r.tryRebind() {
				r.tryResume()
			}
		},
		OnFailure: func(err error, attached bool) {
			if r.constructing {
				r.failure, r.attached = err, attached
				return
			}
			r.fail(err, attached)
		},
	})
	r.browser, r.constructing = b, false
	if err != nil || r.failure != nil {
		if r.failure != nil {
			err = r.failure
		}
		r.fail(err, r.attached)
		return
	}
	if r.readyPending {
		r.readyPending = false
		r.outputReady()
	}
}

func (r *browserWorker) outputReady() {
	if r.phase != workerStarting || r.browser == nil {
		return
	}
	r.attached = true
	if !r.out.CommitWorkerOutput() {
		r.fail(errors.New("audio worker: output attachment failed"), true)
		return
	}
	if r.deadline != nil {
		r.deadline.Stop()
	}
	r.phase = workerActive
	if r.out.WorkerSilent() {
		r.reconcile(r.now())
	} else {
		r.gate(false)
	}
}

// reconcile inserts skip boundaries at frame/update times. Queuing is allowed
// while quiescence is in flight, but Browser sends them only after the flush.
func (r *browserWorker) reconcile(now time.Time) bool {
	silent := r.out.WorkerSilent()
	if silent && !r.clock.started {
		if !r.gate(true) || !r.clock.start(now, r.m.spec.SampleRate) {
			r.fail(errors.New("audio worker: invalid quiet clock"), true)
			return false
		}
		if r.phase == workerActive {
			r.phase = workerQuiescing
		} else if r.phase == workerResuming {
			r.requiesce = true
		}
		if err := r.browser.Quiesce(); err != nil {
			r.fail(err, true)
			return false
		}
		if err := r.browser.QueueSkip(0); err != nil {
			r.fail(err, true)
			return false
		}
	}
	if r.clock.started {
		frames, ok := r.clock.advance(now)
		if !ok {
			r.fail(errors.New("audio worker: invalid quiet time"), true)
			return false
		}
		if frames > 0 || !silent {
			if err := r.browser.QueueSkip(frames); err != nil {
				r.fail(err, true)
				return false
			}
		}
		if !silent {
			r.clock.started = false
		}
	}
	return true
}

func (r *browserWorker) tryResume() {
	if r.phase != workerQuiet || r.clock.started || r.out.WorkerSilent() || !r.browser.Idle() ||
		(r.pending != nil && r.pending.waitQuiet) {
		return
	}
	r.phase = workerResuming
	if err := r.browser.Resume(); err != nil && r.phase == workerResuming {
		r.fail(err, true)
	}
}

func (r *browserWorker) outputResumed() {
	if r.phase != workerResuming {
		return
	}
	if r.requiesce {
		r.requiesce = false
		r.phase = workerQuiescing
		return
	}
	r.phase = workerActive
	if r.out.WorkerSilent() {
		r.reconcile(r.now())
	} else {
		r.gate(false)
	}
}

func (r *browserWorker) Stats() pump.Stats {
	s := pump.Stats{Dropped: r.m.mixer.Commands().Dropped(), Voices: r.m.mixer.Active()}
	if r.hasStats {
		s.Blocks, s.Underruns, s.MixMax = r.stats.Blocks, r.stats.Underruns, r.stats.MixMax
		if r.stats.Blocks > 0 {
			s.MixAvg = r.stats.MixTotal / time.Duration(r.stats.Blocks)
		}
	}
	return s
}

func (r *browserWorker) Reopen(want platform.AudioSpec) error {
	if r == nil || r.pending != nil || r.phase == workerStarting || r.phase == workerRebinding ||
		r.phase == workerFailed || r.phase == workerClosed || r.openOutput == nil {
		return errWorkerReopen
	}
	next := r.openOutput(want)
	if next == nil {
		return errWorkerReopen
	}
	// OpenAudio publishes its new page hooks immediately. Keep the old output
	// selected until the replacement is ready and has inherited its counters.
	if !r.out.SelectWorkerOutput() {
		next.Close()
		return errWorkerReopen
	}
	out, ok := next.(browserWorkerOutput)
	if !ok || !out.ReserveWorkerOutput() {
		next.Close()
		return errWorkerReopen
	}
	if !out.SetWorkerGate(true) || !out.SetWorkerStateListener(r.Tick) {
		out.CancelWorkerOutput()
		out.Close()
		return errWorkerReopen
	}
	p := &pendingWorkerOutput{out: out, want: want}
	r.pending = p
	p.deadline = time.AfterFunc(workerStartupTimeout, func() {
		if r.pending != p {
			return
		}
		if p.waitQuiet || r.phase == workerRebinding {
			r.fail(errors.New("audio worker: reopen timed out"), true)
		} else {
			r.abandonPending(errors.New("audio worker: replacement output timed out"))
			r.tryResume()
		}
	})
	r.Tick()
	return nil
}

// tryRebind first quiesces the old worklet and waits for its final page-side
// counter report. The new worklet starts quiet, so no PCM is rendered across
// a rate change until the pending silent interval has been applied.
func (r *browserWorker) tryRebind() bool {
	p := r.pending
	if p == nil || r.phase == workerStarting || r.phase == workerRebinding || r.phase == workerFailed || r.phase == workerClosed {
		return false
	}
	if message := p.out.WorkerError(); message != "" {
		r.abandonPending(errors.New(message))
		return false
	}
	node, data, index, block, target, ready := p.out.WorkerOutput()
	if !ready {
		return p.waitQuiet
	}
	if !p.waitQuiet {
		p.waitQuiet = true
		if !r.gate(true) {
			return true
		}
		switch r.phase {
		case workerActive:
			r.phase = workerQuiescing
		case workerResuming:
			r.requiesce = true
		}
		if err := r.browser.Quiesce(); err != nil {
			r.fail(err, true)
		}
		return true
	}
	token, quiet := r.browser.QuietToken()
	if r.phase != workerQuiet || !quiet || !r.out.WorkerQuiet(token) || !r.browser.Idle() {
		return true
	}
	if r.clock.started && !p.cut {
		frames, ok := r.clock.advance(r.now())
		if !ok {
			r.fail(errors.New("audio worker: invalid reopen time"), true)
			return true
		}
		p.cut = true
		if err := r.browser.QueueSkip(frames); err != nil {
			r.fail(err, true)
		}
		return true
	}
	spec := p.out.Spec()
	if !p.out.SetWorkerBaseline(r.out.WorkerCounters()) || !p.out.CommitWorkerOutput() || !p.out.SelectWorkerOutput() {
		r.abandonPending(errors.New("audio worker: replacement output could not be prepared"))
		return false
	}
	old := r.out
	old.SetWorkerStateListener(nil)
	r.out = p.out
	r.out.SetWorkerStateListener(r.outputChanged)
	r.m.out, r.m.spec, r.m.requested = p.out, spec, p.want
	r.m.drive(p.out, spec)
	r.m.pump.SetOutput(p.out, pumpConfig(p.out, spec))
	r.gated = true
	if r.clock.started && !r.clock.setRate(spec.SampleRate) {
		r.fail(errors.New("audio worker: invalid replacement rate"), true)
		return true
	}
	r.phase = workerRebinding
	old.Close()
	if err := r.browser.Rebind(workerproto.BrowserOutput{Node: node, Data: data, Index: index,
		Block: block, Target: target, Rate: spec.SampleRate, Quiet: true}); err != nil {
		r.fail(err, true)
	}
	return true
}

func (r *browserWorker) outputRebound() {
	p := r.pending
	if r.phase != workerRebinding || p == nil {
		return
	}
	if p.deadline != nil {
		p.deadline.Stop()
	}
	r.pending = nil
	r.m.mixer.SetSampleRate(r.m.spec.SampleRate)
	r.phase = workerQuiet
	now := r.now()
	if r.clock.started {
		frames, ok := r.clock.advance(now)
		if !ok {
			r.fail(errors.New("audio worker: invalid replacement quiet time"), true)
			return
		}
		if !r.out.WorkerSilent() {
			r.clock.started = false
		}
		if err := r.browser.QueueSkip(frames); err != nil {
			r.fail(err, true)
			return
		}
	} else if r.out.WorkerSilent() {
		if !r.clock.start(now, r.m.spec.SampleRate) {
			r.fail(errors.New("audio worker: invalid replacement clock"), true)
			return
		}
		if err := r.browser.QueueSkip(0); err != nil {
			r.fail(err, true)
			return
		}
	}
	r.tryResume()
}

func (r *browserWorker) abandonPending(err error) {
	p := r.pending
	if p == nil || r.phase == workerRebinding {
		return
	}
	r.pending = nil
	if p.deadline != nil {
		p.deadline.Stop()
	}
	p.out.SetWorkerStateListener(nil)
	p.out.CancelWorkerOutput()
	p.out.Close()
	r.out.SelectWorkerOutput()
	if r.m.log != nil {
		r.m.log.Warn("audio worker output could not be reopened", log.F("error", err.Error()))
	}
}

// fail permits local fallback only before any output attachment and only if
// the controller confirms that no command, skip or batch crossed the bridge.
func (r *browserWorker) fail(err error, attached bool) {
	if r.phase == workerClosed || r.phase == workerFailed {
		return
	}
	if err == nil {
		err = workerproto.ErrBrowserTransport
	}
	if r.deadline != nil {
		r.deadline.Stop()
	}
	if r.pending != nil {
		if r.phase != workerRebinding {
			r.abandonPending(err)
		} else {
			if r.pending.deadline != nil {
				r.pending.deadline.Stop()
			}
			r.pending = nil
		}
	}
	attached = attached || r.attached
	if r.browser != nil {
		r.browser.Terminate()
	} else if r.worker.Truthy() {
		func() {
			defer func() { _ = recover() }()
			r.worker.Call("terminate")
		}()
	}
	if !attached {
		cancelled := r.browser == nil || r.browser.CancelUnsent() == nil
		if cancelled && r.out.CancelWorkerOutput() {
			r.phase = workerClosed
			if r.m.worker == r {
				r.m.worker = nil
				r.m.startPump()
			}
			return
		}
	}
	r.failure, r.phase = err, workerFailed
	r.out.FailWorkerOutput(fmt.Sprintf("audio worker: %v", err))
	r.out.SetWorkerStateListener(nil)
	r.m.endPreview()
	for _, s := range r.m.worlds {
		s.drop(true, 0)
	}
	r.abortAfterTermination(0)
}

func (r *browserWorker) abortAfterTermination(attempt int) {
	if r.browser == nil {
		return
	}
	if err := r.browser.AbortAfterTermination(); errors.Is(err, mix.ErrRemoteStreamBusy) && attempt < 500 {
		time.AfterFunc(10*time.Millisecond, func() { r.abortAfterTermination(attempt + 1) })
	}
}

func (r *browserWorker) Close() {
	if r == nil || r.phase == workerClosed {
		return
	}
	if r.deadline != nil {
		r.deadline.Stop()
	}
	if r.pending != nil {
		if r.phase != workerRebinding {
			r.abandonPending(errWorkerReopen)
		} else {
			if r.pending.deadline != nil {
				r.pending.deadline.Stop()
			}
			r.pending = nil
		}
	}
	r.out.SetWorkerStateListener(nil)
	r.out.SetWorkerGate(true)
	if r.browser != nil {
		r.browser.Terminate()
	} else if r.worker.Truthy() {
		func() {
			defer func() { _ = recover() }()
			r.worker.Call("terminate")
		}()
	}
	r.phase = workerClosed
	r.abortAfterTermination(0)
}
