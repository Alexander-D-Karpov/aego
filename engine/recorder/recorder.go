package recorder

import (
	"strconv"
	"strings"

	"aego/core/ids"
)

type Kind uint8

const (
	Signal Kind = iota
	Event
	Fault
	Spawn
	Despawn
	SceneChange
	Input
	Animation
	Custom
	KindCount
)

const All = uint32(1)<<KindCount - 1

var kindNames = [KindCount]string{"signal", "event", "fault", "spawn", "despawn", "scene", "action", "animation", "custom"}

func (k Kind) String() string {
	if k < KindCount {
		return kindNames[k]
	}
	return "?"
}

func Bit(k Kind) uint32 {
	return 1 << k
}

type Record struct {
	World    uint64
	Frame    uint64
	Tick     uint64
	Kind     Kind
	Owner    ids.NodeID
	Subject  ids.NodeID
	Name     string
	Detail   string
	Replayed bool
}

func short(id ids.NodeID) string {
	return id.String()[:8]
}

func (r *Record) Line(name func(ids.NodeID) string) string {
	if name == nil {
		name = short
	}
	var b strings.Builder
	b.WriteString("frame " + strconv.FormatUint(r.Frame, 10) + "  tick " + strconv.FormatUint(r.Tick, 10) + "  " + r.Kind.String())
	if !r.Owner.IsZero() {
		b.WriteString("  " + name(r.Owner))
	}
	if r.Name != "" {
		b.WriteString("  " + r.Name)
	}
	if !r.Subject.IsZero() {
		b.WriteString("  → " + name(r.Subject))
	}
	if r.Detail != "" {
		b.WriteString("  " + r.Detail)
	}
	return b.String()
}

type Recorder struct {
	ring      []Record
	head      int
	n         int
	counts    [KindCount]uint64
	mask      uint32
	nodes     map[ids.NodeID]bool
	world     uint64
	frame     uint64
	tick      uint64
	rev       uint64
	brMask    uint32
	brText    string
	tripped   bool
	trip      Record
	replaying bool
}

func New(size int) *Recorder {
	if size <= 0 {
		size = 4096
	}
	return &Recorder{ring: make([]Record, size), mask: All}
}

func (r *Recorder) Stamp(world, frame, tick uint64) {
	r.world, r.frame, r.tick = world, frame, tick
}

func (r *Recorder) SetFilter(mask uint32, nodes []ids.NodeID) {
	r.mask = mask
	if len(nodes) == 0 {
		r.nodes = nil
		return
	}
	r.nodes = make(map[ids.NodeID]bool, len(nodes))
	for _, n := range nodes {
		r.nodes[n] = true
	}
}

func (r *Recorder) Watching(node ids.NodeID) bool {
	return r.nodes != nil && r.nodes[node]
}

func (r *Recorder) Wants(k Kind) bool {
	return r != nil && r.mask&Bit(k) != 0
}

func (r *Recorder) Count(k Kind) {
	if r != nil && k < KindCount {
		r.counts[k]++
	}
}

func (r *Recorder) SetReplaying(on bool) {
	if r != nil {
		r.replaying = on
	}
}

func (r *Recorder) Add(k Kind, owner, subject ids.NodeID, name, detail string) {
	if r == nil || k >= KindCount {
		return
	}
	r.counts[k]++
	if r.mask&Bit(k) == 0 || (r.nodes != nil && !r.nodes[owner] && !r.nodes[subject]) {
		return
	}
	rec := Record{World: r.world, Frame: r.frame, Tick: r.tick, Kind: k, Owner: owner, Subject: subject, Name: name, Detail: detail, Replayed: r.replaying}
	if r.n < len(r.ring) {
		r.ring[(r.head+r.n)%len(r.ring)] = rec
		r.n++
	} else {
		r.ring[r.head] = rec
		r.head = (r.head + 1) % len(r.ring)
	}
	r.rev++
	if !r.replaying && !r.tripped && r.brMask&Bit(k) != 0 && (r.brText == "" || strings.Contains(name, r.brText) || strings.Contains(detail, r.brText)) {
		r.tripped, r.trip = true, rec
	}
}

func (r *Recorder) BreakOn(mask uint32, text string) {
	if mask == r.brMask && text == r.brText {
		return
	}
	r.brMask, r.brText, r.tripped = mask, text, false
}

func (r *Recorder) Tripped() (Record, bool) {
	if !r.tripped {
		return Record{}, false
	}
	r.tripped = false
	return r.trip, true
}

func (r *Recorder) Snapshot(dst []Record) []Record {
	dst = dst[:0]
	for i := 0; i < r.n; i++ {
		dst = append(dst, r.ring[(r.head+i)%len(r.ring)])
	}
	return dst
}

func (r *Recorder) Counts() [KindCount]uint64 {
	return r.counts
}

func (r *Recorder) Revision() uint64 {
	return r.rev
}

func (r *Recorder) Clear() {
	r.head, r.n = 0, 0
	r.counts = [KindCount]uint64{}
	r.rev++
}
