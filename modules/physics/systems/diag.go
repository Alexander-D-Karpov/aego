package systems

import (
	"strings"

	"aego/core/ids"
	"aego/core/log"
	"aego/modules/physics/components"
	"aego/scene"
)

const (
	CodeScale    = "PHY001"
	CodeGeometry = "PHY002"
	CodeBody     = "PHY003"
	CodeJoint    = "PHY004"
	CodeSpace    = "PHY005"
	CodeReparent = "PHY006"
	CodeUpdate   = "PHY007"
)

const (
	keyBody     = "body"
	keySpace    = "space"
	keyReparent = "reparent"
	keyUpdate   = "update"
)

type Diag struct {
	Node      ids.NodeID
	Path      string
	Code      string
	Message   string
	Error     bool
	Component string
	h         scene.NodeHandle
	key       string
}

func componentOf(key string) string {
	for _, p := range [...]string{"shape/", "joint/"} {
		if c, ok := strings.CutPrefix(key, p); ok {
			return c
		}
	}
	return ""
}

func shapeKeyName(k components.Kind) string {
	return "shape/" + components.Names[k]
}

func jointKeyName(k jointKind) string {
	if k == jointDistance {
		return "joint/" + components.NameDistance
	}
	return "joint/" + components.NameRevolute
}

func (m *Sim) report(h scene.NodeHandle, key, code, msg string, isErr bool) {
	for i := range m.diags {
		d := &m.diags[i]
		if d.h != h || d.key != key {
			continue
		}
		if d.Code == code && d.Message == msg && d.Error == isErr {
			return
		}
		d.Code, d.Message, d.Error = code, msg, isErr
		m.diagRev++
		m.logDiag(d)
		return
	}
	m.diags = append(m.diags, Diag{Node: m.s.ID(h), Path: m.s.Path(h), Code: code, Message: msg, Error: isErr, Component: componentOf(key), h: h, key: key})
	m.diagRev++
	m.logDiag(&m.diags[len(m.diags)-1])
}

func (m *Sim) logDiag(d *Diag) {
	fields := []log.Field{log.F("node", d.Path), log.F("code", d.Code)}
	if d.Error {
		m.log.Error(d.Message, fields...)
		return
	}
	m.log.Warn(d.Message, fields...)
}

func (m *Sim) resolve(h scene.NodeHandle, key string) {
	for i := range m.diags {
		if m.diags[i].h == h && m.diags[i].key == key {
			m.diags = append(m.diags[:i], m.diags[i+1:]...)
			m.diagRev++
			return
		}
	}
}

func (m *Sim) forget(h scene.NodeHandle) {
	n := 0
	for _, d := range m.diags {
		if d.h != h || m.s.IsAlive(h) {
			m.diags[n] = d
			n++
		}
	}
	if n != len(m.diags) {
		m.diags = m.diags[:n]
		m.diagRev++
	}
}

func (m *Sim) Diagnostics() ([]Diag, uint64) {
	return m.diags, m.diagRev
}
