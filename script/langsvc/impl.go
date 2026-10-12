package langsvc

import (
	"path/filepath"
	"strconv"
	"strings"
)

type Impl struct {
	Path string
	Line int
}

func (s *Service) SetNativeRoot(dir string) {
	s.smu.Lock()
	s.native = dir
	s.smu.Unlock()
}

func (s *Service) nativeRoot() string {
	s.smu.Lock()
	defer s.smu.Unlock()
	return s.native
}

func (s *Service) Implementation(def Location) (Impl, bool) {
	if !def.Native || def.Stub == "" {
		return Impl{}, false
	}
	return s.StubImplementation(def.Stub, def.Symbol)
}

func (s *Service) StubImplementation(stub, symbol string) (Impl, bool) {
	st, ok := s.Stub(stub)
	if !ok {
		return Impl{}, false
	}
	return projectImpl(st.Locs[symbol], s.nativeRoot())
}

func projectImpl(loc, root string) (Impl, bool) {
	if loc == "" || root == "" {
		return Impl{}, false
	}
	i := strings.LastIndexByte(loc, ':')
	if i <= 0 {
		return Impl{}, false
	}
	line, err := strconv.Atoi(loc[i+1:])
	if err != nil || line <= 0 {
		return Impl{}, false
	}
	file := filepath.Clean(loc[:i])
	rel, err := filepath.Rel(filepath.Clean(root), file)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return Impl{}, false
	}
	return Impl{Path: file, Line: line}, true
}
