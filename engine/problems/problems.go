package problems

import (
	"strconv"

	"aego/core/ids"
	"aego/scene"
)

const ServiceID = "engine.problems"

type Severity uint8

const (
	Info Severity = iota
	Warning
	Error
)

type SubjectKind uint8

const (
	SubjectNone SubjectKind = iota
	SubjectNode
	SubjectField
	SubjectAsset
	SubjectSetting
	SubjectFile
)

type Subject struct {
	Kind    SubjectKind
	Scene   *scene.Scene
	Node    ids.NodeID
	Field   string
	Asset   ids.AssetID
	Setting string
	File    string
	Line    int
	Col     int
}

type Diagnostic struct {
	Owner      string
	Code       string
	Severity   Severity
	Message    string
	Help       string
	Subject    Subject
	BlocksPlay bool
}

func (d *Diagnostic) Key() string {
	s := &d.Subject
	return d.Owner + "|" + d.Code + "|" + strconv.Itoa(int(s.Kind)) + "|" + s.Node.String() + "|" + s.Field + "|" +
		s.Asset.String() + "|" + s.Setting + "|" + s.File + ":" + strconv.Itoa(s.Line) + ":" + strconv.Itoa(s.Col)
}

type Source struct {
	Revision func() uint64
	Append   func(dst []Diagnostic) []Diagnostic
}

// entry caches what its source appended at a revision, so Collect asks again only the sources that changed (D346).
type entry struct {
	id     uint64
	src    Source
	cur    uint64
	rev    uint64
	cached []Diagnostic
	ok     bool
}

type Service struct {
	entries []entry
	next    uint64
	gen     uint64
	rev     uint64
	valid   bool
	list    []Diagnostic
	seen    map[string]bool
}

func New() *Service {
	return &Service{seen: map[string]bool{}}
}

func (s *Service) Add(src Source) func() {
	s.next++
	id := s.next
	s.entries = append(s.entries, entry{id: id, src: src})
	s.gen++
	return func() {
		for i := range s.entries {
			if s.entries[i].id == id {
				s.entries = append(s.entries[:i], s.entries[i+1:]...)
				s.gen++
				return
			}
		}
	}
}

func (s *Service) revision() (uint64, bool) {
	h := (s.gen ^ 14695981039346656037) * 1099511628211
	force := false
	for i := range s.entries {
		e := &s.entries[i]
		if e.src.Revision == nil {
			force = true
			continue
		}
		e.cur = e.src.Revision()
		h = (h ^ e.cur) * 1099511628211
	}
	return h, force
}

func (s *Service) Collect() ([]Diagnostic, uint64) {
	if s == nil {
		return nil, 0
	}
	rev, force := s.revision()
	if s.valid && rev == s.rev && !force {
		return s.list, rev
	}
	s.list = s.list[:0]
	clear(s.seen)
	for i := range s.entries {
		e := &s.entries[i]
		if e.src.Append == nil {
			continue
		}
		if e.src.Revision == nil || !e.ok || e.rev != e.cur {
			e.cached = e.src.Append(e.cached[:0])
			e.rev, e.ok = e.cur, e.src.Revision != nil
		}
		start := len(s.list)
		s.list = append(s.list, e.cached...)
		n := start
		for i := start; i < len(s.list); i++ {
			k := s.list[i].Key()
			if s.seen[k] {
				continue
			}
			s.seen[k] = true
			s.list[n] = s.list[i]
			n++
		}
		s.list = s.list[:n]
	}
	s.rev, s.valid = rev, true
	return s.list, rev
}

func (s *Service) Blocking() (Diagnostic, int) {
	list, _ := s.Collect()
	var first Diagnostic
	n := 0
	for i := range list {
		if list[i].BlocksPlay && list[i].Severity == Error {
			if n == 0 {
				first = list[i]
			}
			n++
		}
	}
	return first, n
}
