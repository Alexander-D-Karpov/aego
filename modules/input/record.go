package input

import (
	"math"
	"strconv"
	"strings"

	"aego/core/errs"
	"aego/core/mathx"
)

const recordHeader = "aeinput 1"

type RecordMeta struct {
	Project string
	Seed    uint64
	FixedHz float64
}

type frameSample struct {
	frame                   uint32
	slot                    uint32
	down, pressed, released bool
	v                       mathx.Vec2
}

type Recording struct {
	meta    RecordMeta
	start   uint64
	frames  uint32
	players int
	actions []string
	samples []frameSample
	last    []frameSample
}

type replay struct {
	samples []frameSample
	next    int
	start   uint64
}

func (s *Service) actionNames() []string {
	out := make([]string, len(s.cfg.Actions))
	for i, a := range s.cfg.Actions {
		out[i] = a.Name
	}
	return out
}

func (s *Service) Record(m RecordMeta) {
	s.rec = &Recording{meta: m, start: s.frame, players: s.players, actions: s.actionNames(), last: make([]frameSample, len(s.states))}
}

func (s *Service) Recording() bool {
	return s.rec != nil
}

func (s *Service) StopRecording() []byte {
	r := s.rec
	s.rec = nil
	if r == nil {
		return nil
	}
	return r.Encode()
}

func (r *Recording) capture(s *Service) {
	f := uint32(s.frame - r.start)
	r.frames = f
	for i := range s.states {
		st := &s.states[i]
		l := &r.last[i]
		if !st.pressed && !st.released && st.down == l.down && st.value == l.v {
			continue
		}
		x := frameSample{frame: f, slot: uint32(i), down: st.down, pressed: st.pressed, released: st.released, v: st.value}
		r.samples = append(r.samples, x)
		*l = x
	}
}

func flags(x frameSample) uint64 {
	var f uint64
	if x.down {
		f |= 1
	}
	if x.pressed {
		f |= 2
	}
	if x.released {
		f |= 4
	}
	return f
}

func (r *Recording) Encode() []byte {
	b := []byte(recordHeader + "\n")
	b = append(b, "project "+r.meta.Project+"\n"...)
	b = append(b, "seed "...)
	b = strconv.AppendUint(b, r.meta.Seed, 10)
	b = append(b, "\nfixedHz "...)
	b = strconv.AppendFloat(b, r.meta.FixedHz, 'g', -1, 64)
	b = append(b, "\nplayers "...)
	b = strconv.AppendInt(b, int64(r.players), 10)
	b = append(b, "\nactions "+strings.Join(r.actions, " ")+"\nframes "...)
	b = strconv.AppendUint(b, uint64(r.frames), 10)
	b = append(b, '\n')
	for _, x := range r.samples {
		b = append(b, "s "...)
		b = strconv.AppendUint(b, uint64(x.frame), 10)
		b = append(b, ' ')
		b = strconv.AppendUint(b, uint64(x.slot), 10)
		b = append(b, ' ')
		b = strconv.AppendUint(b, flags(x), 10)
		b = append(b, ' ')
		b = strconv.AppendUint(b, uint64(math.Float32bits(x.v.X)), 16)
		b = append(b, ' ')
		b = strconv.AppendUint(b, uint64(math.Float32bits(x.v.Y)), 16)
		b = append(b, '\n')
	}
	return b
}

func recErr(msg string) error {
	return errs.New(errs.EngineModule, "input recording: "+msg)
}

func DecodeRecording(data []byte) (*Recording, error) {
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != recordHeader {
		return nil, recErr("not an aeinput version 1 file")
	}
	r := &Recording{}
	for i, l := range lines[1:] {
		f := strings.Fields(l)
		if len(f) == 0 {
			continue
		}
		var err error
		switch f[0] {
		case "project":
			r.meta.Project = strings.TrimSpace(strings.TrimPrefix(l, "project"))
		case "seed":
			r.meta.Seed, err = strconv.ParseUint(f[1], 10, 64)
		case "fixedHz":
			r.meta.FixedHz, err = strconv.ParseFloat(f[1], 64)
		case "players":
			r.players, err = strconv.Atoi(f[1])
		case "actions":
			r.actions = f[1:]
		case "frames":
			var n uint64
			n, err = strconv.ParseUint(f[1], 10, 32)
			r.frames = uint32(n)
		case "s":
			if len(f) != 6 {
				return nil, recErr("line " + strconv.Itoa(i+2) + " is malformed")
			}
			var x frameSample
			fr, e1 := strconv.ParseUint(f[1], 10, 32)
			sl, e2 := strconv.ParseUint(f[2], 10, 32)
			fl, e3 := strconv.ParseUint(f[3], 10, 8)
			xb, e4 := strconv.ParseUint(f[4], 16, 32)
			yb, e5 := strconv.ParseUint(f[5], 16, 32)
			for _, e := range []error{e1, e2, e3, e4, e5} {
				if e != nil {
					return nil, recErr("line " + strconv.Itoa(i+2) + ": " + e.Error())
				}
			}
			x.frame, x.slot = uint32(fr), uint32(sl)
			x.down, x.pressed, x.released = fl&1 != 0, fl&2 != 0, fl&4 != 0
			x.v = mathx.Vec2{X: math.Float32frombits(uint32(xb)), Y: math.Float32frombits(uint32(yb))}
			r.samples = append(r.samples, x)
		default:
			return nil, recErr("unknown line " + f[0])
		}
		if err != nil {
			return nil, recErr(err.Error())
		}
	}
	return r, nil
}

func (s *Service) Replay(data []byte) (RecordMeta, error) {
	r, err := DecodeRecording(data)
	if err != nil {
		return RecordMeta{}, err
	}
	if r.players != s.players {
		return r.meta, recErr("recorded with " + strconv.Itoa(r.players) + " players, the project has " + strconv.Itoa(s.players))
	}
	oldN, n := len(r.actions), len(s.cfg.Actions)
	remap := make([]int, oldN)
	for i, name := range r.actions {
		ai, ok := s.acts[name]
		if !ok {
			return r.meta, recErr("names action " + name + ", which the project does not declare")
		}
		remap[i] = ai
	}
	samples := make([]frameSample, len(r.samples))
	for i, x := range r.samples {
		if oldN == 0 || int(x.slot) >= oldN*r.players {
			return r.meta, recErr("sample slot is out of range")
		}
		p, a := int(x.slot)/oldN, int(x.slot)%oldN
		x.slot = uint32(p*n + remap[a])
		samples[i] = x
	}
	for i := range s.states {
		st := &s.states[i]
		*st = state{pressAt: math.Inf(-1)}
	}
	s.inj = s.inj[:0]
	s.play = &replay{samples: samples, start: s.frame}
	return r.meta, nil
}

func (s *Service) Replaying() bool {
	return s.play != nil
}

func (s *Service) StopReplay() {
	s.play = nil
}

func (s *Service) replayFrame() {
	r := s.play
	f := uint32(s.frame - r.start)
	for r.next < len(r.samples) && r.samples[r.next].frame <= f {
		x := r.samples[r.next]
		st := &s.states[x.slot]
		st.down, st.value = x.down, x.v
		st.pressed, st.released = x.pressed, x.released
		r.next++
	}
}
