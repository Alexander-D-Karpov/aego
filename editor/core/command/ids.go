package command

import "aego/platform/input"

const (
	FileSave        = "file.save"
	FileSaveAs      = "file.save_as"
	FileSaveAll     = "file.save_all"
	FileOpen        = "file.open"
	FileNewScene    = "file.new_scene"
	EditUndo        = "edit.undo"
	EditRedo        = "edit.redo"
	EditCopy        = "edit.copy"
	EditCut         = "edit.cut"
	EditPaste       = "edit.paste"
	EditDuplicate   = "edit.duplicate"
	EditDelete      = "edit.delete"
	EditRename      = "edit.rename"
	EditSelectAll   = "edit.select_all"
	SceneCreateNode = "scene.create_node"
	SceneFrame      = "scene.frame_selection"
	SceneToolSelect = "scene.tool.select"
	SceneToolMove   = "scene.tool.translate"
	SceneToolRotate = "scene.tool.rotate"
	SceneToolScale  = "scene.tool.scale"
	SceneToggleGrid = "scene.toggle_grid"
	SceneToggleSnap = "scene.toggle_snap"
	PlayStart       = "play.start"
	PlayPause       = "play.pause"
	PlayStep        = "play.step"
	PlayStepFixed   = "play.step_fixed"
	PlayScalePrefix = "play.scale."
	PlayStop        = "play.stop"
	AssetsReimport  = "assets.reimport"
	CommandPalette  = "command.palette"
	WindowPrefix    = "window."
	ScopeScene      = "scene"
	ScopeHierarchy  = "hierarchy"
	ScopeAssets     = "assets"
)

func DefaultShortcuts() map[string]Shortcut {
	return map[string]Shortcut{
		FileSave:        Key(input.KeyS, ModPrimary),
		FileSaveAs:      Key(input.KeyS, ModPrimary|ModShift),
		FileSaveAll:     Key(input.KeyS, ModPrimary|ModAlt),
		FileOpen:        Key(input.KeyO, ModPrimary),
		FileNewScene:    Key(input.KeyN, ModPrimary),
		EditUndo:        Key(input.KeyZ, ModPrimary),
		EditRedo:        Key(input.KeyZ, ModPrimary|ModShift),
		EditCopy:        Key(input.KeyC, ModPrimary),
		EditCut:         Key(input.KeyX, ModPrimary),
		EditPaste:       Key(input.KeyV, ModPrimary),
		EditDuplicate:   Key(input.KeyD, ModPrimary),
		EditDelete:      Key(input.KeyDelete, 0),
		EditRename:      Key(input.KeyF2, 0),
		EditSelectAll:   Key(input.KeyA, ModPrimary),
		SceneFrame:      Key(input.KeyF, 0),
		SceneToolSelect: Key(input.KeyQ, 0),
		SceneToolMove:   Key(input.KeyW, 0),
		SceneToolRotate: Key(input.KeyE, 0),
		SceneToolScale:  Key(input.KeyR, 0),
		SceneToggleSnap: Key(input.KeyG, ModPrimary),
		PlayStart:       Key(input.KeyP, ModPrimary),
		PlayPause:       Key(input.KeyP, ModPrimary|ModShift),
		PlayStep:        Key(input.KeyP, ModPrimary|ModAlt),
		PlayStepFixed:   Key(input.KeyP, ModAlt),
		CommandPalette:  Key(input.KeyP, ModPrimary|ModShift|ModAlt),
	}
}

func ScopeOf(id string) string {
	switch id {
	case SceneFrame, SceneToolSelect, SceneToolMove, SceneToolRotate, SceneToolScale:
		return ScopeScene
	}
	return ""
}
