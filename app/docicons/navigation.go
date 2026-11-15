// Package docicons holds the built-in documentation icon policy at the
// application composition boundary. Modules and editor panels do not depend on
// this policy; it is supplied to the documentation registry by the host.
package docicons

import "maps"

// These maps mirror tools/gendocs/site/icons.go; the parity test checks every
// key and value, including module-owned guides, API packages and settings.
var taskIcons = map[string]string{
	"install": "file.download", "quickstart": "doc.project", "platformer": "doc.tutorial",
	"platformer-effects": "render2d.point_light", "credits": "help.about", "recipes": "doc.recipe",
	"editor": "view.dock", "editor/agent": "panel.agent", "agents": "panel.agent",
	"editor/icons": "view.grid", "cli": "panel.console", "diagnostics": "panel.problems",
	"curves": "ui.curve", "tween": "code.timer", "animation": "animation.player",
	"animation/animator": "animation.animator", "animation/controller": "anim.graph",
	"animation/clips": "anim.key", "animation/events": "anim.event", "animation/time": "play.speed",
	"tween/timers": "code.timer", "render2d/materials": "file.preset",
	"render2d/normal-mapping": "render2d.normals", "render2d/art": "file.art",
	"render2d/image-viewer": "file.texture", "render2d.PostProcess": "view.overlays",
	"camera.CameraBrain2D": "view.split", "formats/overview": "file.data",
	"formats/aeanim": "anim.key", "formats/aectrl": "anim.graph",
}

var refIcons = map[string]string{
	"aego/builtin": "cat.node", "events": "panel.events", "aego/action": "panel.tasks", "aego/audio": "cat.audio",
	"aego/camera": "cat.camera", "aego/curve": "ui.curve", "aego/data": "file.data", "aego/debug": "panel.debug_draw",
	"aego/input": "panel.input_devices", "aego/log": "panel.console", "aego/math": "cat.transform", "aego/physics": "cat.physics",
	"aego/rand": "shape.dice", "aego/save": "panel.saves", "aego/scene": "file.scene", "aego/settings": "cat.settings",
	"aego/testing": "status.ok", "aego/time": "play.speed", "aego/timer": "code.timer", "aego/tween": "vec.velocity",
	"section/engine": "panel.settings", "section/input": "panel.input_devices", "section/render2d": "cat.rendering",
	"section/physics": "cat.physics", "section/debug": "panel.debug_draw", "section/player": "play.start", "section/audio": "cat.audio",
	"section/animation": "animation.player", "section/art": "file.art", "section/update": "file.download",
}

// Builtin returns independent task and reference maps for a host to install.
func Builtin() (tasks, references map[string]string) {
	return maps.Clone(taskIcons), maps.Clone(refIcons)
}
