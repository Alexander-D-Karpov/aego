package state

import (
	"os"

	"aego/editor/core/document"
)

const WorkspacesFile = "workspaces.aetext"

func LoadWorkspaces(root string) ([]byte, bool) {
	data, err := os.ReadFile(userPath(root, WorkspacesFile))
	return data, err == nil
}

func SaveWorkspaces(root string, data []byte) error {
	return document.WriteAtomic(userPath(root, WorkspacesFile), data)
}
