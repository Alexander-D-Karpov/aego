package abi

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// errFrozen is returned by every Add* once Freeze has sealed the table.
var errFrozen = errors.New("abi: table is frozen")

type Table struct {
	modules map[string]bool
	funcs   map[string]Signature
	methods map[string]Signature
	consts  map[string]Const
	enums   map[string]*EnumDef
	comps   []*Component
	hash    [32]byte
	frozen  bool
}

func NewTable() *Table {
	return &Table{
		modules: map[string]bool{},
		funcs:   map[string]Signature{},
		methods: map[string]Signature{},
		consts:  map[string]Const{},
		enums:   map[string]*EnumDef{},
	}
}

func key(a, b string) string {
	return a + "\x00" + b
}

func (t *Table) AddModule(path string) error {
	if t.frozen {
		return errFrozen
	}
	t.modules[path] = true
	return nil
}

func (t *Table) AddFunc(module, name string, sig Signature) error {
	if t.frozen {
		return errFrozen
	}
	if err := sig.Validate(); err != nil {
		return fmt.Errorf("func %s.%s: %w", module, name, err)
	}
	k := key(module, name)
	if _, dup := t.funcs[k]; dup {
		return fmt.Errorf("func %s.%s already registered", module, name)
	}
	t.modules[module] = true
	t.funcs[k] = sig
	return nil
}

func (t *Table) AddMethod(recv Kind, name string, sig Signature) error {
	if t.frozen {
		return errFrozen
	}
	if err := sig.Validate(); err != nil {
		return fmt.Errorf("method %s.%s: %w", recv, name, err)
	}
	k := key(recv.String(), name)
	if _, dup := t.methods[k]; dup {
		return fmt.Errorf("method %s.%s already registered", recv, name)
	}
	t.methods[k] = sig
	return nil
}

func (t *Table) AddConst(module, name string, c Const) error {
	if t.frozen {
		return errFrozen
	}
	k := key(module, name)
	if _, dup := t.consts[k]; dup {
		return fmt.Errorf("const %s.%s already registered", module, name)
	}
	t.modules[module] = true
	t.consts[k] = c
	return nil
}

func (t *Table) AddEnum(e *EnumDef) error {
	if t.frozen {
		return errFrozen
	}
	k := key(e.Module, e.Name)
	if _, dup := t.enums[k]; dup {
		return fmt.Errorf("enum %s already registered", e.Qualified())
	}
	t.modules[e.Module] = true
	t.enums[k] = e
	return nil
}

func (t *Table) AddComponent(c *Component) error {
	if t.frozen {
		return errFrozen
	}
	if c.Short == "" {
		c.Short = c.Name[strings.LastIndexByte(c.Name, '.')+1:]
	}
	for _, e := range t.comps {
		if e.Name == c.Name {
			return fmt.Errorf("component %s already registered", c.Name)
		}
	}
	for i := range c.Fields {
		c.Fields[i].Index = i
	}
	c.Index = len(t.comps)
	t.comps = append(t.comps, c)
	return nil
}

func (t *Table) Module(path string) bool {
	return t.modules[path]
}

func (t *Table) Func(module, name string) (Signature, bool) {
	s, ok := t.funcs[key(module, name)]
	return s, ok
}

func (t *Table) Method(recv Kind, name string) (Signature, bool) {
	s, ok := t.methods[key(recv.String(), name)]
	return s, ok
}

func (t *Table) Const(module, name string) (Const, bool) {
	c, ok := t.consts[key(module, name)]
	return c, ok
}

func (t *Table) Enum(module, name string) (*EnumDef, bool) {
	e, ok := t.enums[key(module, name)]
	return e, ok
}

func (t *Table) Components(name string) []*Component {
	for _, c := range t.comps {
		if c.Name == name {
			return []*Component{c}
		}
	}
	var out []*Component
	for _, c := range t.comps {
		if c.Short == name {
			out = append(out, c)
		}
	}
	return out
}

func (t *Table) AllComponents() []*Component {
	return t.comps
}

func (t *Table) Hash() [32]byte {
	if !t.frozen {
		panic("abi: Hash called on an unfrozen table; call Freeze first")
	}
	return t.hash
}

// Freeze seals the table and computes its hash. It is final: every later Add*
// returns errFrozen. Calling it again is a no-op.
func (t *Table) Freeze() {
	if t.frozen {
		return
	}
	var lines []string
	for p := range t.modules {
		lines = append(lines, "p|"+p)
	}
	for k, s := range t.funcs {
		lines = append(lines, "f|"+k+"|"+s.String())
	}
	for k, s := range t.methods {
		lines = append(lines, "m|"+k+"|"+s.String())
	}
	for k, c := range t.consts {
		lines = append(lines, "c|"+k+"|"+c.Type.String()+"|"+strconv.FormatInt(c.Int, 10)+"|"+
			strconv.FormatFloat(c.Float, 'g', -1, 64)+"|"+strconv.FormatBool(c.Bool)+"|"+c.Str)
	}
	for k, e := range t.enums {
		var b strings.Builder
		b.WriteString("e|" + k)
		for _, m := range e.Members {
			b.WriteString("|" + m.Name + "=" + strconv.FormatInt(m.Value, 10))
		}
		lines = append(lines, b.String())
	}
	for _, c := range t.comps {
		var b strings.Builder
		b.WriteString("k|" + c.Name)
		for _, f := range c.Fields {
			b.WriteString("|" + f.Name + ":" + f.Type.String() + ":" + strconv.FormatBool(f.ReadOnly))
		}
		lines = append(lines, b.String())
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l))
		h.Write([]byte{'\n'})
	}
	copy(t.hash[:], h.Sum(nil))
	t.frozen = true
}
