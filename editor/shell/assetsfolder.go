package shell

import (
	"aego/editor/core/workspace"
	"aego/editor/panels/assets"
)

// workspaceFolders is where Assets opens in a workspace the first time; later it reopens where it was left.
var workspaceFolders = map[string]string{workspace.Programming: "scripts"}

func (a *App) syncAssetsFolder() {
	if a.assetsPanel == nil {
		return
	}
	cur := a.ws.Current().ID
	if cur == a.folderWS {
		return
	}
	if a.folders == nil {
		a.folders = map[string]string{}
	}
	if a.folderWS != "" {
		a.folders[a.folderWS] = a.assetsPanel.Folder()
	}
	a.folderWS = cur
	dir, ok := a.folders[cur]
	if !ok {
		dir = workspaceFolders[cur]
	}
	if dir == "" {
		dir = "assets"
	}
	a.assetsPanel.SetFolder(dir)
}

// AssetsPanel is the Assets panel, for tests.
func (a *App) AssetsPanel() *assets.Panel {
	return a.assetsPanel
}
