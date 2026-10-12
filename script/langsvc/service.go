package langsvc

import (
	"sync"
	"sync/atomic"

	"aego/script/abi"
	"aego/script/bind/manifest"
	"aego/script/diag"
	"aego/script/source"
	"aego/script/workspace"
)

type Jobs interface {
	Submit(fn func())
	OnMain(fn func())
}

type Result struct {
	Snap        *workspace.Snapshot
	Diagnostics []diag.Diagnostic
	Tokens      []Token
}

type Service struct {
	ws       *workspace.Workspace
	jobs     Jobs
	gen      atomic.Uint64
	mu       sync.Mutex
	last     *workspace.Snapshot
	smu      sync.Mutex
	stubTab  *abi.Table
	stubDocs *manifest.Manifest
	stubs    map[string]*Stub
	native   string
}

func New(w *workspace.Workspace, jobs Jobs) *Service {
	return &Service{ws: w, jobs: jobs}
}

func (s *Service) Workspace() *workspace.Workspace {
	return s.ws
}

func (s *Service) Current() uint64 {
	return s.gen.Load()
}

func (s *Service) Invalidate() uint64 {
	return s.gen.Add(1)
}

func (s *Service) Refresh() *workspace.Snapshot {
	snap := s.ws.Snapshot()
	s.mu.Lock()
	if s.last == nil || snap.Gen >= s.last.Gen {
		s.last = snap
	}
	s.mu.Unlock()
	return snap
}

func (s *Service) Latest() *workspace.Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

func (s *Service) deliver(gen uint64, fn func()) {
	s.jobs.OnMain(func() {
		if s.Current() == gen {
			fn()
		}
	})
}

func (s *Service) Analyze(gen uint64, file source.FileID, fn func(Result)) {
	s.jobs.Submit(func() {
		snap := s.Refresh()
		r := Result{Snap: snap, Diagnostics: snap.Diagnostics, Tokens: semanticTokens(snap, file)}
		s.deliver(gen, func() { fn(r) })
	})
}

func (s *Service) Diagnostics(gen uint64, fn func([]diag.Diagnostic)) {
	s.jobs.Submit(func() {
		ds := s.Refresh().Diagnostics
		s.deliver(gen, func() { fn(ds) })
	})
}

func (s *Service) Complete(gen uint64, file source.FileID, off uint32, fn func(Completion)) {
	s.jobs.Submit(func() {
		c := s.complete(s.Refresh(), file, off)
		s.deliver(gen, func() { fn(c) })
	})
}

func (s *Service) CompleteNow(file source.FileID, off uint32) Completion {
	return s.complete(s.Refresh(), file, off)
}
