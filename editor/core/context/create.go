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
}
