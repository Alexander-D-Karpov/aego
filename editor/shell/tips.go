package shell

import (
	"aego/editor/core/command"
	"aego/editor/panels/assets"
	"aego/editor/panels/inspector"
	"aego/editor/panels/search"
)

var commandInfos = []command.Info{
	{ID: command.FileNewScene, Icon: "file.scene", Tip: "Open a new unsaved 2D scene with a camera"},
	{ID: CommandNewScene3D, Icon: "file.scene", Tip: "Open a new unsaved 3D scene with a camera, a plane and a cube"},
	{ID: command.FileOpen, Icon: "file.folder", Tip: "Pick a scene of the project and open it"},
	{ID: command.FileSave, Icon: "file.save", Tip: "Save the active document"},
	{ID: command.FileSaveAs, Icon: "file.save", Tip: "Save the active document under a new path"},
	{ID: command.FileSaveAll, Icon: "file.save", Tip: "Save all documents with unsaved changes"},
	{ID: CommandCloseScene, Icon: "ui.close", Tip: "Close the active document, asking first if it has unsaved changes"},
	{ID: CommandQuit, Icon: "", Tip: "Quit the editor, asking first about unsaved changes"},
	{ID: command.EditUndo, Icon: "nav.back", Tip: "Undo the last change of the active document"},
	{ID: command.EditRedo, Icon: "nav.forward", Tip: "Redo the change you undid last"},
	{ID: command.EditCut, Icon: "", Tip: "Copy the selected content and delete it"},
	{ID: command.EditCopy, Icon: "", Tip: "Copy the selected content to the clipboard"},
	{ID: command.EditPaste, Icon: "", Tip: "Paste clipboard content into the active document"},
	{ID: command.EditDuplicate, Icon: "", Tip: "Copy the selected nodes next to themselves"},
	{ID: command.EditDelete, Icon: "", Tip: "Delete the selected content of the active document"},
	{ID: command.EditRename, Icon: "", Tip: "Rename the selected node in the Hierarchy"},
	{ID: command.EditSelectAll, Icon: "", Tip: "Select all content of the active document"},
	{ID: command.SceneToolSelect, Icon: "tool.select", Tip: "Click to select, drag to select with a box"},
	{ID: command.SceneToolMove, Icon: "tool.move", Tip: "Drag the selection or one of its axes"},
	{ID: command.SceneToolRotate, Icon: "tool.rotate", Tip: "Rotate the selection around its pivot"},
	{ID: command.SceneToolScale, Icon: "tool.scale", Tip: "Scale the selection along its axes"},
	{ID: command.SceneFrame, Icon: "view.zoom", Tip: "Centre the view on the selection"},
	{ID: command.SceneToggleGrid, Icon: "view.grid", Tip: "Show the grid in the Scene View"},
	{ID: command.SceneToggleSnap, Icon: "view.snap", Tip: "Snap moves, rotations and scales to steps; hold Ctrl to invert"},
	{ID: CommandToggleOrtho, Icon: "", Tip: "Switch the 3D Scene View between perspective and orthographic"},
	{ID: command.SceneCreateNode, Icon: "ui.plus", Tip: "Create an empty node under the selection"},
	{ID: command.PlayStart, Icon: "play.start", Tip: "Run the active scene in the Game View, or stop it"},
	{ID: command.PlayPause, Icon: "play.pause", Tip: "Pause or resume game time"},
	{ID: command.PlayStep, Icon: "play.step", Tip: "Run one frame while paused"},
	{ID: command.PlayReleaseInput, Icon: "tool.select", Tip: "Return keyboard and mouse to the editor; the game keeps Escape"},
	{ID: command.PlayStepFixed, Icon: "play.step", Tip: "Run one fixed tick while paused"},
	{ID: command.PlayStop, Icon: "play.stop", Tip: "Stop Play and return to the edit scene"},
	{ID: command.AssetsReimport, Icon: "ui.reset", Tip: "Import the selected asset again"},
	{ID: CommandAssetsRename, Icon: "", Tip: "Rename the selected asset; its id and references stay"},
	{ID: CommandAssetsDuplicate, Icon: "", Tip: "Copy the selected assets with new ids"},
	{ID: CommandAssetsDelete, Icon: "", Tip: "Delete the selected assets and their .meta files"},
	{ID: CommandAssetsUndo, Icon: "nav.back", Tip: "Undo the last file operation in Assets"},
	{ID: CommandAssetsRedo, Icon: "nav.forward", Tip: "Redo the file operation you undid last"},
	{ID: assets.CommandCopy, Icon: "", Tip: "Copy the selected assets"},
	{ID: assets.CommandCut, Icon: "", Tip: "Cut the selected assets to move them on Paste"},
	{ID: assets.CommandPaste, Icon: "", Tip: "Paste copied or cut assets into the open folder"},
	{ID: inspector.CommandCopy, Icon: "", Tip: "Copy the field or component under the cursor"},
	{ID: inspector.CommandPaste, Icon: "", Tip: "Paste into the field or component under the cursor"},
	{ID: inspector.CommandWatch, Icon: "", Tip: "Add the field under the cursor to the Watch panel"},
	{ID: search.CommandOpen, Icon: "ui.search", Tip: "Find files, symbols, nodes and actions"},
	{ID: CommandResetLayout, Icon: "view.dock", Tip: "Put every panel back to its default place"},
	{ID: CommandRepairMeta, Icon: "status.warning", Tip: "Write missing .meta files and remove orphans after confirmation"},
	{ID: CommandGettingStarted, Icon: "help.book", Tip: "Open the Getting Started checklist"},
	{ID: CommandReference, Icon: "help.book", Tip: "Open the documentation of the language and the engine"},
	{ID: CommandKeymap, Icon: "", Tip: "List every keyboard shortcut"},
	{ID: command.PlayScalePrefix + "custom", Icon: "", Tip: "Type any time scale from 0 to 8"},
}

func (a *App) describeCommands() {
	a.ctx.Commands.DescribeAll(commandInfos)
}
