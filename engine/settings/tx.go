package settings

import (
	"strconv"

	"aego/schema"
)

type change struct {
	k     Key
	v     schema.Value
	reset bool
}

type Tx struct {
	s    *Store
	list []change
	done bool
}

func (s *Store) Begin() *Tx {
	return &Tx{s: s}
}

func (t *Tx) find(k Key) int {
	for i := range t.list {
		if t.list[i].k == k {
			return i
		}
	}
	return -1
}

func (t *Tx) put(c change) {
	if i := t.find(c.k); i >= 0 {
		t.list[i] = c
		return
	}
	t.list = append(t.list, c)
}

func (t *Tx) Set(k Key, v schema.Value) error {
	e := t.s.at(k)
	switch {
	case e == nil:
		return bad("#"+strconv.Itoa(int(k)), "does not exist")
	case !e.d.Flags.Has(User):
		return bad(e.d.ID, "is not a player setting")
	}
	n, err := t.s.check(e, v)
	if err != nil {
		return err
	}
	t.put(change{k: k, v: n})
	return nil
}

func (t *Tx) SetID(id string, v schema.Value) error {
	k := t.s.Key(id)
	if k == 0 {
		return bad(id, "does not exist")
	}
	return t.Set(k, v)
}

func (t *Tx) Reset(k Key) {
	if t.s.at(k) != nil {
		t.put(change{k: k, reset: true})
	}
}

func (t *Tx) Value(k Key) (schema.Value, bool) {
	if i := t.find(k); i >= 0 && !t.list[i].reset {
		return t.list[i].v, true
	}
	return schema.Value{}, false
}

func (t *Tx) Len() int {
	return len(t.list)
}

func (t *Tx) Done() bool {
	return t.done
}

func (t *Tx) Revert() {
	t.list = nil
	t.done = true
}

func (t *Tx) Apply() error {
	return t.apply(0)
}

func (t *Tx) ApplyWithConfirm(seconds float64) error {
	return t.apply(seconds)
}

func (t *Tx) apply(confirm float64) error {
	s := t.s
	if t.done || len(t.list) == 0 {
		t.done = true
		return nil
	}
	t.done = true
	t.expandPreset()
	keys := make([]Key, 0, len(t.list)+1)
	for _, c := range t.list {
		keys = append(keys, c.k)
	}
	if s.preset != 0 && t.find(s.preset) < 0 {
		keys = append(keys, s.preset)
	}
	snap := s.snapshot(keys)
	err := s.commit(keys, func() {
		for _, c := range t.list {
			e := s.at(c.k)
			if c.reset {
				e.user, e.hasUser = schema.Value{}, false
			} else {
				e.user, e.hasUser = c.v, true
			}
		}
		s.matchPreset(t)
	})
	if err == nil && confirm > 0 {
		s.confirm = &pending{deadline: s.now + confirm, snap: snap}
	}
	return err
}

func (t *Tx) expandPreset() {
	s := t.s
	i := t.find(s.preset)
	if s.preset == 0 || i < 0 || t.list[i].reset {
		return
	}
	c, ok := s.choiceOf(s.at(s.preset), t.list[i].v)
	if !ok || c.Name == PresetCustom {
		return
	}
	for j := range s.list {
		e := &s.list[j]
		k := Key(j + 1)
		pv, ok := e.d.Presets[c.Name]
		if !ok || t.find(k) >= 0 {
			continue
		}
		if n, err := s.check(e, pv); err == nil {
			t.list = append(t.list, change{k: k, v: n})
		}
	}
}

func (s *Store) presetMatches(name string) bool {
	found := false
	for i := range s.list {
		e := &s.list[i]
		pv, ok := e.d.Presets[name]
		if !ok {
			continue
		}
		found = true
		n, _ := Normalize(e.d.Kind, pv)
		if v, _ := e.value(); !Equal(v, n) {
			return false
		}
	}
	return found
}

func (s *Store) matchPreset(t *Tx) {
	if s.preset == 0 || t.find(s.preset) >= 0 {
		return
	}
	touched := false
	for _, c := range t.list {
		if len(s.at(c.k).d.Presets) > 0 {
			touched = true
		}
	}
	if !touched {
		return
	}
	pe := s.at(s.preset)
	pick := PresetCustom
	for _, c := range pe.d.Choices {
		if c.Name != PresetCustom && s.presetMatches(c.Name) {
			pick = c.Name
			break
		}
	}
	for _, c := range pe.d.Choices {
		if c.Name == pick {
			pe.user, pe.hasUser = c.Value, true
		}
	}
}
