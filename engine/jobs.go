package engine

import (
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

type Jobs struct {
	work    chan func()
	wg      sync.WaitGroup
	mu      sync.Mutex
	main    []func()
	spare   []func()
	workers int
	stopped bool
	onPanic atomic.Pointer[func(any, []byte)]
}

func newJobs(workers int) *Jobs {
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0) - 1
	}
	if workers < 1 {
		workers = 1
	}
	j := &Jobs{
		work:    make(chan func(), 1024),
		main:    make([]func(), 0, 64),
		spare:   make([]func(), 0, 64),
		workers: workers,
	}
	for i := 0; i < workers; i++ {
		j.wg.Go(j.loop)
	}
	return j
}

func (j *Jobs) SetPanicHandler(fn func(rec any, stack []byte)) {
	j.onPanic.Store(&fn)
}

func (j *Jobs) loop() {
	for fn := range j.work {
		j.run(fn)
	}
}

func (j *Jobs) run(fn func()) {
	defer func() {
		if rec := recover(); rec != nil {
			if h := j.onPanic.Load(); h != nil {
				(*h)(rec, debug.Stack())
			}
			panic(rec)
		}
	}()
	fn()
}

func (j *Jobs) Workers() int {
	return j.workers
}

func (j *Jobs) Submit(fn func()) {
	j.work <- fn
}

func (j *Jobs) OnMain(fn func()) {
	j.mu.Lock()
	j.main = append(j.main, fn)
	j.mu.Unlock()
}

func (j *Jobs) drain() {
	j.mu.Lock()
	run := j.main
	j.main = j.spare[:0]
	j.mu.Unlock()
	for i, fn := range run {
		fn()
		run[i] = nil
	}
	j.spare = run[:0]
}

func (j *Jobs) stop() {
	if j.stopped {
		return
	}
	j.stopped = true
	close(j.work)
	j.wg.Wait()
	j.drain()
}
