package main

import "strings"

// Group kinds; every package belongs to exactly one group by path prefix.
const (
	kLow       = "low"
	kKernel    = "kernel"
	kModule    = "module"
	kModEditor = "module-editor"
	kEdCore    = "editor-core"
	kEdExt     = "editor-ext"
	kPanel     = "panel"
	kShell     = "editor-shell"
	kSDK       = "sdk"
	kApp       = "app"
	kRuntime   = "runtime"
	kCmd       = "cmd"
	kTool      = "tool"
	kDemo      = "demo"
	kTest      = "test"
	kExample   = "example"
)

type group struct {
	kind string
	name string // module or panel name
}

var lowRoots = []string{"core", "platform", "gpu", "format", "vfs", "update"}
var kernelRoots = []string{"schema", "scene", "render", "asset", "engine", "script", "ui", "project", "contracts", "data", "save", "audio", "anim"}

func groupOf(p string) (group, bool) {
	seg := strings.Split(p, "/")
	switch seg[0] {
	case "modules":
		if len(seg) < 2 {
			return group{}, false
		}
		if len(seg) >= 3 && seg[2] == "editor" {
			return group{kModEditor, seg[1]}, true
		}
		return group{kModule, seg[1]}, true
	case "editor":
		if len(seg) < 2 {
			return group{}, false
		}
		switch seg[1] {
		case "core":
			return group{kind: kEdCore}, true
		case "ext":
			return group{kind: kEdExt}, true
		case "shell":
			return group{kind: kShell}, true
		case "panels":
			if len(seg) < 3 {
				return group{}, false
			}
			return group{kPanel, seg[2]}, true
		}
		return group{}, false
	case "sdk":
		return group{kind: kSDK}, true
	case "app":
		return group{kind: kApp}, true
	case "runtime":
		return group{kind: kRuntime}, true
	case "cmd":
		return group{kind: kCmd}, true
	case "tools":
		return group{kind: kTool}, true
	case "demos":
		return group{kind: kDemo}, true
	case "tests":
		return group{kind: kTest}, true
	case "examples":
		return group{kind: kExample}, true
	}
	for _, r := range lowRoots {
		if seg[0] == r {
			return group{kind: kLow}, true
		}
	}
	for _, r := range kernelRoots {
		if seg[0] == r {
			return group{kind: kKernel}, true
		}
	}
	return group{}, false
}

func isEditor(g group) bool {
	switch g.kind {
	case kEdCore, kEdExt, kPanel, kShell:
		return true
	}
	return false
}

// groupRule returns the group rule an import breaks, or "".
func groupRule(from, to group) string {
	switch from.kind {
	case kLow, kKernel:
		switch {
		case to.kind == kModule || to.kind == kModEditor:
			return "kernel-imports-module"
		case isEditor(to):
			return "kernel-imports-editor"
		case to.kind == kSDK:
			return "kernel-imports-sdk"
		case to.kind == kApp:
			return "kernel-imports-app"
		case to.kind == kRuntime:
			return "kernel-imports-runtime"
		}
	case kModule, kModEditor:
		switch {
		case (to.kind == kModule || to.kind == kModEditor) && to.name != from.name:
			return "module-imports-module"
		case from.kind == kModule && to.kind == kModEditor:
			return "module-imports-own-editor"
		case from.kind == kModule && isEditor(to):
			return "module-imports-editor"
		case from.kind == kModEditor && isEditor(to) && to.kind != kEdCore && to.kind != kEdExt:
			return "module-editor-imports-editor-app"
		case to.kind == kSDK:
			return "module-imports-sdk"
		case to.kind == kApp:
			return "module-imports-app"
		case to.kind == kRuntime:
			return "module-imports-runtime"
		}
	case kEdCore:
		switch to.kind {
		case kPanel, kEdExt, kShell:
			return "editor-core-imports-" + to.kind
		}
	case kEdExt:
		switch to.kind {
		case kPanel, kShell:
			return "editor-ext-imports-" + to.kind
		}
	case kPanel:
		switch {
		case to.kind == kPanel && to.name != from.name:
			return "panel-imports-panel"
		case to.kind == kShell:
			return "panel-imports-" + to.kind
		}
	case kRuntime:
		if isEditor(to) || to.kind == kModEditor {
			return "runtime-imports-editor"
		}
	}
	if isEditor(from) && to.kind == kRuntime {
		return "editor-imports-runtime"
	}
	if isEditor(from) && from.kind != kShell && (to.kind == kModule || to.kind == kModEditor) {
		return "editor-imports-module"
	}
	if from.kind == kSDK && isEditor(to) {
		return "sdk-imports-editor"
	}
	if from.kind == kSDK && to.kind == kModEditor {
		return "sdk-imports-module-editor"
	}
	if from.kind == kSDK && to.kind == kRuntime {
		return "sdk-imports-runtime"
	}
	return ""
}

// runtimeForbidden lists what the aego_runtime build must never reach.
var runtimeForbidden = with(scriptCompilerOnly, "asset/pipeline", "asset/importers/...", "project", "project/...", "editor/...",
	"app", "app/agent/...", "app/agentui/...", "app/bootstrap", "app/docstool", "app/edit", "app/edit/...", "app/export", "app/export/...", "app/playground",
	"app/projects", "app/run", "app/supervise", "app/assembly/editors", "app/assembly/playground",
	"engine/docs", "cmd/aego", "cmd/aego-editor", "cmd/aego-playground", "cmd/sandbox",
	"demos/...", "tests/...", "tools/...", "examples/...",
	"modules/*/editor/...", "modules/*/dev/...", "scene/prefab/author")

// wasmDenied lists what the playground (js/wasm) must not reach.
var wasmDenied = []string{
	"gpu/gl33/...", "platform/sdl", "asset/pipeline", "asset/importers/...",
	"modules/*/editor/...", "modules/*/dev/...",
}

var wasmExtra = []string{
	"modules/render2d", "modules/scripting/...", "modules/input", "modules/physics/...", "modules/debug", "modules/tween", "modules/animation", "modules/camera", "modules/gameplay", "modules/render3d",
	"modules/spawn", "modules/savegame", "modules/audio", "app/assembly", "app/assembly/playground", "app/playground", "cmd/aego-playground",
}

// match reports whether p matches pattern: "a/b", "a/...", "..." or "*" per segment.
func match(pattern, p string) bool {
	if pattern == "..." {
		return true
	}
	if base, ok := strings.CutSuffix(pattern, "/..."); ok {
		return matchSegs(base, p, true)
	}
	return matchSegs(pattern, p, false)
}

func matchSegs(pattern, p string, prefix bool) bool {
	ps, xs := strings.Split(pattern, "/"), strings.Split(p, "/")
	if len(xs) < len(ps) || (!prefix && len(xs) != len(ps)) {
		return false
	}
	for i, s := range ps {
		if s != "*" && s != xs[i] {
			return false
		}
	}
	return true
}

func allowed(patterns []string, p string) bool {
	for _, pat := range patterns {
		if match(pat, p) {
			return true
		}
	}
	return false
}
