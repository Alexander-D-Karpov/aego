package shell

import (
	"aego/editor/core/command"
	"aego/editor/panels/assets"
	"aego/editor/panels/inspector"
	"aego/editor/panels/search"
)

type commandInfo struct {
	icon string
	tip  string
}

var commandInfos = map[string]commandInfo{
	command.FileNewScene:               {"file.scene", "Open a new unsaved 2D scene with a camera"},
	CommandNewScene3D:                  {"file.scene", "Open a new unsaved 3D scene with a camera, a plane and a cube"},
	command.FileOpen:                   {"file.folder", "Pick a scene of the project and open it"},
	command.FileSave:                   {"file.save", "Save the active scene or prefab"},
	command.FileSaveAs:                 {"file.save", "Save the active scene under a new path"},
	command.FileSaveAll:                {"file.save", "Save every scene, prefab and script with unsaved changes"},
	CommandCloseScene:                  {"ui.close", "Close the active scene, asking first if it has unsaved changes"},
	CommandQuit:                        {"", "Quit the editor, asking first about unsaved changes"},
	command.EditUndo:                   {"nav.back", "Undo the last change of the active scene"},
	command.EditRedo:                   {"nav.forward", "Redo the change you undid last"},
	command.EditCut:                    {"", "Copy the selected nodes and delete them"},
	command.EditCopy:                   {"", "Copy the selected nodes to the clipboard"},
	command.EditPaste:                  {"", "Paste nodes from the clipboard under the selection"},
	command.EditDuplicate:              {"", "Copy the selected nodes next to themselves"},
	command.EditDelete:                 {"", "Delete the selected nodes and their children"},
	command.EditRename:                 {"", "Rename the selected node in the Hierarchy"},
	command.EditSelectAll:              {"", "Select every node of the active scene"},
	command.SceneToolSelect:            {"tool.select", "Click to select, drag to select with a box"},
	command.SceneToolMove:              {"tool.move", "Drag the selection or one of its axes"},
	command.SceneToolRotate:            {"tool.rotate", "Rotate the selection around its pivot"},
	command.SceneToolScale:             {"tool.scale", "Scale the selection along its axes"},
	command.SceneFrame:                 {"view.zoom", "Centre the view on the selection"},
	command.SceneToggleGrid:            {"view.grid", "Show the grid in the Scene View"},
	command.SceneToggleSnap:            {"view.snap", "Snap moves, rotations and scales to steps; hold Ctrl to invert"},
	CommandToggleOrtho:                 {"", "Switch the 3D Scene View between perspective and orthographic"},
	command.SceneCreateNode:            {"ui.plus", "Create an empty node under the selection"},
	command.PlayStart:                  {"play.start", "Run the active scene in the Game View, or stop it"},
	command.PlayPause:                  {"play.pause", "Pause or resume game time"},
	command.PlayStep:                   {"play.step", "Run one frame while paused"},
	command.PlayReleaseInput:           {"tool.select", "Return keyboard and mouse to the editor; the game keeps Escape"},
	command.PlayStepFixed:              {"play.step", "Run one fixed tick while paused"},
	command.PlayStop:                   {"play.stop", "Stop Play and return to the edit scene"},
	command.AssetsReimport:             {"ui.reset", "Import the selected asset again"},
	CommandAssetsRename:                {"", "Rename the selected asset; its id and references stay"},
	CommandAssetsDuplicate:             {"", "Copy the selected assets with new ids"},
	CommandAssetsDelete:                {"", "Delete the selected assets and their .meta files"},
	CommandAssetsUndo:                  {"nav.back", "Undo the last file operation in Assets"},
	CommandAssetsRedo:                  {"nav.forward", "Redo the file operation you undid last"},
	assets.CommandCopy:                 {"", "Copy the selected assets"},
	assets.CommandCut:                  {"", "Cut the selected assets to move them on Paste"},
	assets.CommandPaste:                {"", "Paste copied or cut assets into the open folder"},
	inspector.CommandCopy:              {"", "Copy the field or component under the cursor"},
	inspector.CommandPaste:             {"", "Paste into the field or component under the cursor"},
	inspector.CommandWatch:             {"", "Add the field under the cursor to the Watch panel"},
	search.CommandOpen:                 {"ui.search", "Find files, symbols, nodes and actions"},
	CommandResetLayout:                 {"view.dock", "Put every panel back to its default place"},
	CommandRepairMeta:                  {"status.warning", "Write missing .meta files and remove orphans after confirmation"},
	CommandGettingStarted:              {"help.book", "Open the Getting Started checklist"},
	CommandReference:                   {"help.book", "Open the documentation of the language and the engine"},
	CommandKeymap:                      {"", "List every keyboard shortcut"},
	command.PlayScalePrefix + "custom": {"", "Type any time scale from 0 to 8"},
}

func (a *App) describeCommands() {
	for id, in := range commandInfos {
		a.ctx.Commands.Describe(id, in.icon, in.tip)
	}
}
