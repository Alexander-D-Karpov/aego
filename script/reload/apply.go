package reload

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"aego/schema"
	"aego/script/abi"
	"aego/script/runtime"
	"aego/script/vm"
)

type Report struct {
	From         uint64
	Generation   uint64
	Migrated     int
	Reset        int
	Killed       int
	Canceled     int
	Disconnected int
	Elapsed      time.Duration
}

func (r Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Hot reload %d → %d: %d migrated, %d reset", r.From, r.Generation, r.Migrated, r.Reset)
	if r.Killed > 0 {
		fmt.Fprintf(&b, ", %d stopped", r.Killed)
	}
	fmt.Fprintf(&b, ", %d coroutines canceled, %d handlers disconnected, %.1f ms",
		r.Canceled, r.Disconnected, float64(r.Elapsed.Microseconds())/1000)
	return b.String()
}

type converter struct {
	m    *vm.Machine
	from *layout
	to   *layout
}

func (c *converter) value(a, b abi.TypeRef, src, dst []vm.Value) bool {
	switch classify(a, b, c.from, c.to) {
	case Same, Reresolve:
		copy(dst, src)
		return true
	case Struct:
		return c.structValue(a, b, src, dst)
	case EnumByName:
		old, cur := c.from.enums[a.Name], c.to.enums[b.Name]
		i := src[0].Int()
		if i < 0 || int(i) >= len(old) {
			return false
		}
		j := slices.Index(cur, old[i])
		if j < 0 {
			return false
		}
		dst[0] = vm.Int(int64(j))
		return true
	case Slice:
		return c.sliceValue(a, b, src, dst)
	}
	return false
}

func (c *converter) structValue(a, b abi.TypeRef, src, dst []vm.Value) bool {
	os, ns := c.from.structs[a.Name], c.to.structs[b.Name]
	for _, nf := range ns.Fields {
		of := structField(os, nf.Name)
		if of == nil || int(of.Slot)+int(of.Slots) > len(src) || int(nf.Slot)+int(nf.Slots) > len(dst) {
			continue
		}
		ot, ok1 := abi.Parse(of.Type)
		nt, ok2 := abi.Parse(nf.Type)
		if !ok1 || !ok2 {
			continue
		}
		d := dst[nf.Slot : int(nf.Slot)+int(nf.Slots)]
		tmp := append([]vm.Value(nil), d...)
		if c.value(ot, nt, src[of.Slot:int(of.Slot)+int(of.Slots)], tmp) {
			copy(d, tmp)
		}
	}
	return true
}

func (c *converter) sliceValue(a, b abi.TypeRef, src, dst []vm.Value) bool {
	if src[0].A == 0 {
		dst[0] = vm.Value{}
		return true
	}
	elems, w, ok := c.m.SliceData(src[0])
	if !ok || w == 0 {
		return false
	}
	ae, be := *a.Elem, *b.Elem
	nw := c.to.slots(be)
	n := len(elems) / w
	out := make([]vm.Value, n*nw)
	for k := 0; k < n; k++ {
		if !c.value(ae, be, elems[k*w:(k+1)*w], out[k*nw:(k+1)*nw]) {
			return false
		}
	}
	dst[0] = c.m.MakeSlice(nw, c.to.mask(be), out)
	return true
}

func (p *Plan) migrate(c *converter, newName string, src, dst []vm.Value) int {
	tc := p.byName[newName]
	if tc == nil || tc.Old == nil {
		return 0
	}
	reset := 0
	for _, fm := range tc.Fields {
		if fm.OldSlot+fm.OldSlots > len(src) || fm.NewSlot+fm.NewSlots > len(dst) {
			reset++
			continue
		}
		s := src[fm.OldSlot : fm.OldSlot+fm.OldSlots]
		d := dst[fm.NewSlot : fm.NewSlot+fm.NewSlots]
		switch fm.Conv {
		case Same, Reresolve:
			copy(d, s)
		case Default:
			reset++
		default:
			tmp := append([]vm.Value(nil), d...)
			if c.value(fm.Old, fm.New, s, tmp) {
				copy(d, tmp)
			} else {
				reset++
			}
		}
	}
	return reset
}

func Apply(r *runtime.Runtime, reg *schema.Registry, p *Plan) (Report, error) {
	start := time.Now()
	rep := Report{From: p.From.ID, Generation: p.To.ID}
	sw := r.BeginSwap(reg)
	if err := sw.LoadModules(p.To.Ordered()); err != nil {
		sw.Abort()
		return rep, err
	}
	c := &converter{m: r.Machine(), from: p.from, to: p.to}
	migrated, reset, killed, err := sw.Migrate(func(_, newName string, src, dst []vm.Value) int {
		return p.migrate(c, newName, src, dst)
	})
	if err != nil {
		sw.Abort()
		return rep, err
	}
	again, err := sw.Commit(p.To.ID, p.changed)
	if err != nil {
		sw.Abort()
		return rep, err
	}
	rep.Canceled = r.CancelChanged(p.changed)
	rep.Disconnected = r.RebindSignals()
	r.CallReloaded(again)
	rep.Migrated, rep.Reset, rep.Killed = migrated, reset, killed
	rep.Elapsed = time.Since(start)
	return rep, nil
}
