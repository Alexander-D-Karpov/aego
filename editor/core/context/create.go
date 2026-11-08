package context

import "aego/core/ids"

// CreateAction extends the existing searchable Create palette from a module.
type CreateAction struct {
	Key      string
	Name     string
	Category string
	Icon     string
	Doc      string
	Run      func(parent ids.NodeID) error
	// RunInFolder makes an asset action available in the Assets folder menu.
	// The folder is project-relative and selected by the user.
	RunInFolder func(dir string) error
}

// ExternalFileImport lets a module convert a selected external source into a
// project asset without teaching the generic file service a module's format.
type ExternalFileImport struct {
	Key        string
	Extensions []string
	// Pending is true when the handler opened a confirmation dialog and has
	// not created an asset yet. It must report the outcome after the decision.
	Import func(source, dir string) (id ids.AssetID, pending bool, err error)
}
