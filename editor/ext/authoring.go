package ext

import "aego/editor/core/workspace"

// CommandsAvailable reports whether this extension registry can register
// commands. Some hosts provide documents and panels without a command service.
func (r *Registry) CommandsAvailable() bool {
	return r.deps.Commands != nil
}

// Presentation queries expose existing docking, not a second layout manager.
func (r *Registry) PanelVisible(id string) bool {
	return r.deps.Workspace != nil && r.deps.Workspace.Visible(id)
}

// MaximizedPanel identifies the only panel drawn while a workspace is
// maximized. PanelVisible still reports panels present in the saved layout.
func (r *Registry) MaximizedPanel() string {
	if r.deps.Workspace == nil {
		return ""
	}
	return r.deps.Workspace.Tree.Maximized()
}
func (r *Registry) CurrentWorkspace() workspace.Preset {
	if r.deps.Workspace == nil {
		return workspace.Preset{}
	}
	return r.deps.Workspace.Current()
}
func (r *Registry) SelectWorkspace(id string) bool {
	return r.deps.Workspace != nil && r.deps.Workspace.Switch(id)
}
