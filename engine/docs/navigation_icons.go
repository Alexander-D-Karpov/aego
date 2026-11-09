//go:build !aego_runtime

package docs

import "maps"

// SetNavigationIcons installs host-provided presentation overrides. It copies
// the maps so a caller cannot change a registry by retaining its inputs.
// Configure a registry before it is used by a documentation view.
func (r *Registry) SetNavigationIcons(tasks, references map[string]string) {
	r.navTasks = maps.Clone(tasks)
	r.navReferences = maps.Clone(references)
}

// NavigationTaskIcon is the task-specific override, before entry metadata.
func (r *Registry) NavigationTaskIcon(id string) string {
	if r == nil {
		return ""
	}
	return r.navTasks[id]
}

// NavigationReferenceIcon is the reference override, after entry metadata.
func (r *Registry) NavigationReferenceIcon(id string) string {
	if r == nil {
		return ""
	}
	return r.navReferences[id]
}
