package ext

import (
	"strings"

	"aego/core/errs"
	"aego/core/log"
	"aego/editor/core/workspace"
)

const workspaceOrder = 100

func (r *Registry) Workspace(p workspace.Preset) error {
	ws := r.deps.Workspace
	switch {
	case r.current == "":
		return errs.New(errs.EditorPanel, "a workspace is registered outside an extension")
	case ws == nil:
		return errs.New(errs.EditorPanel, "workspaces are not available", log.F("module", r.current))
	}
	if !strings.HasPrefix(p.ID, r.current+".") {
		p.ID = r.current + "." + p.ID
	}
	p.Owner, p.User, p.Base = r.current, false, ""
	if p.Order < workspaceOrder {
		p.Order += workspaceOrder
	}
	return ws.RegisterPreset(p)
}
