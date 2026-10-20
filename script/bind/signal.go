package bind

import (
	"errors"
	"fmt"

	"aego/script/abi"
)

var errSignalModule = errors.New("signals can be declared only on the methods module of a handle type")

func Signal(m *Module, name string, params []abi.TypeRef, opts ...Option) {
	if m.handle == "" {
		m.valid(name, errSignalModule)
		return
	}
	for i, p := range params {
		if !abi.Plain(p) {
			m.valid(name, fmt.Errorf("param %d: %s cannot be a signal parameter", i, p))
			return
		}
	}
	if !m.valid(name, m.reg.table.AddHandleSignal(m.handle, abi.SignalDef{Name: name, Params: params})) {
		return
	}
	m.valid(name, m.reg.describe(m.path, name, opts, len(params)))
}
