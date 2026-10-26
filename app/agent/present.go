package agent

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"

	"aego/engine/i18n"
)

// Look is how the editor shows a call of a command: an icon, a title from its arguments and a summary of its result (11b5a).
type Look struct {
	Icon    string
	Title   func(a Args) string
	Summary func(out *Output) string
}

// Present is the look of a tool by its tool name; docs_search, docs_read and ask_user have one too.
func Present(tool string) (Look, bool) {
	for _, c := range all {
		if c.Tool() == tool {
			return c.Look, true
		}
	}
	l, ok := extraLooks[tool]
	return l, ok
}

// DecodeOutput reads a tool result back; Value stays raw JSON for the summaries.
func DecodeOutput(text string) (*Output, bool) {
	var raw struct {
		Output
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, false
	}
	out := raw.Output
	if len(raw.Value) > 0 {
		out.Value = raw.Value
	}
	return &out, true
}

func file(a Args) string {
	return path.Base(a.Str("file"))
}

// nodeName shows a node path the way the hierarchy does: Player › Sprite.
func nodeName(n string) string {
	n = strings.TrimPrefix(strings.TrimPrefix(n, "Root/"), "/")
	if n == "" || n == "Root" {
		return i18n.Tr("editor.agent.step.root", "the root")
	}
	return strings.ReplaceAll(n, "/", " › ")
}

func node(a Args) string {
	return nodeName(a.Str("node"))
}

func items(out *Output) int {
	raw, ok := out.Value.(json.RawMessage)
	if !ok {
		return 0
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil {
		return len(list)
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) == nil {
		for _, k := range []string{"results", "rows", "states"} {
			if json.Unmarshal(m[k], &list) == nil && list != nil {
				return len(list)
			}
		}
		return len(m)
	}
	return 0
}

func nodes(out *Output) int {
	raw, ok := out.Value.(json.RawMessage)
	if !ok {
		return 0
	}
	var v NodeView
	if json.Unmarshal(raw, &v) != nil {
		return 0
	}
	var count func(n *NodeView) int
	count = func(n *NodeView) int {
		c := 1
		for i := range n.Children {
			c += count(&n.Children[i])
		}
		return c
	}
	return count(&v) - 1
}

func lines(out *Output) int {
	raw, ok := out.Value.(json.RawMessage)
	if !ok {
		return 0
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return 0
	}
	return strings.Count(strings.TrimRight(s, "\n"), "\n") + 1
}

func changes(out *Output) string {
	if len(out.Changes) == 1 && (out.Changes[0].Before != "" || out.Changes[0].After != "") && !strings.Contains(out.Changes[0].After, "\n") {
		c := out.Changes[0]
		return clip(c.Before, 24) + " → " + clip(c.After, 24)
	}
	if len(out.Changes) == 0 {
		return i18n.Tr("editor.agent.step.no_changes", "nothing to change")
	}
	return i18n.TrN("editor.agent.step.changes", int64(len(out.Changes)), "{n} change", "{n} changes")
}

func problems(out *Output) string {
	if len(out.Problems) == 0 {
		return i18n.Tr("editor.agent.step.no_problems", "no problems")
	}
	return i18n.TrN("editor.agent.step.problems", int64(len(out.Problems)), "{n} problem", "{n} problems")
}

func count(out *Output) string {
	return i18n.TrN("editor.agent.step.items", int64(items(out)), "{n} item", "{n} items")
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i] + "…"
	}
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

var txLabel = regexp.MustCompile(`label="([^"]*)"`)

func looks() map[string]Look {
	ok := func(*Output) string { return "" }
	return map[string]Look{
		"scene.tree": {"panel.hierarchy", func(a Args) string {
			return i18n.Tr("editor.agent.step.read_scene", "Read scene {file}", i18n.Str("file", file(a)))
		}, func(o *Output) string {
			return i18n.TrN("editor.agent.step.nodes", int64(nodes(o)), "{n} node", "{n} nodes")
		}},
		"scene.get": {"panel.hierarchy", func(a Args) string {
			if p := a.Str("path"); p != "" {
				return i18n.Tr("editor.agent.step.read_field", "Read {target}", i18n.Str("target", node(a)+" › "+p))
			}
			return i18n.Tr("editor.agent.step.read_node", "Read {node}", i18n.Str("node", node(a)))
		}, ok},
		"scene.set": {"edit.set", func(a Args) string {
			return i18n.Tr("editor.agent.step.set", "Set {target}", i18n.Str("target", node(a)+" › "+a.Str("path")))
		}, changes},
		"scene.reset": {"edit.set", func(a Args) string {
			return i18n.Tr("editor.agent.step.reset", "Reset {target}", i18n.Str("target", node(a)+" › "+a.Str("path")))
		}, changes},
		"scene.add-node": {"edit.add", func(a Args) string {
			return i18n.Tr("editor.agent.step.add_node", "Add node {name}", i18n.Str("name", a.Str("name")))
		}, changes},
		"scene.add-comp": {"edit.add", func(a Args) string {
			return i18n.Tr("editor.agent.step.add_comp", "Add {comp} to {node}", i18n.Str("comp", a.Str("comp")), i18n.Str("node", node(a)))
		}, changes},
		"scene.remove-node": {"edit.remove", func(a Args) string {
			return i18n.Tr("editor.agent.step.remove_node", "Remove {node}", i18n.Str("node", node(a)))
		}, changes},
		"scene.remove-comp": {"edit.remove", func(a Args) string {
			return i18n.Tr("editor.agent.step.remove_comp", "Remove {comp} from {node}", i18n.Str("comp", a.Str("comp")), i18n.Str("node", node(a)))
		}, changes},
		"scene.move": {"edit.set", func(a Args) string {
			return i18n.Tr("editor.agent.step.move", "Move {node} under {parent}", i18n.Str("node", node(a)), i18n.Str("parent", nodeName(a.Str("parent"))))
		}, changes},
		"scene.rename": {"edit.rename", func(a Args) string {
			return i18n.Tr("editor.agent.step.rename", "Rename {node} to {name}", i18n.Str("node", node(a)), i18n.Str("name", a.Str("name")))
		}, changes},
		"scene.apply": {"edit.batch", func(a Args) string {
			if m := txLabel.FindStringSubmatch(a.Str("tx")); m != nil && m[1] != "" {
				return i18n.Tr("editor.agent.step.apply_label", "{label} in {file}", i18n.Str("label", m[1]), i18n.Str("file", file(a)))
			}
			return i18n.Tr("editor.agent.step.apply", "Change {file}", i18n.Str("file", file(a)))
		}, changes},
		"scene.new": {"file.scene", func(a Args) string {
			return i18n.Tr("editor.agent.step.new_scene", "Create scene {file}", i18n.Str("file", file(a)))
		}, ok},
		"scene.run": {"play.start", func(a Args) string {
			return i18n.Tr("editor.agent.step.run", "Run {file} headless", i18n.Str("file", file(a)))
		}, func(o *Output) string {
			var r RunReport
			if raw, isRaw := o.Value.(json.RawMessage); isRaw {
				_ = json.Unmarshal(raw, &r)
			}
			parts := []string{i18n.TrN("editor.agent.step.ticks", int64(r.Frames), "{n} tick", "{n} ticks")}
			if len(r.Checks) > 0 {
				ok := 0
				for _, c := range r.Checks {
					if c.OK {
						ok++
					}
				}
				parts = append(parts, i18n.Tr("editor.agent.step.checks", "{ok} of {n} checks hold", i18n.Int("ok", int64(ok)), i18n.Int("n", int64(len(r.Checks)))))
			}
			if len(r.Shots) > 0 {
				parts = append(parts, i18n.TrN("editor.agent.step.shots", int64(len(r.Shots)), "{n} screenshot", "{n} screenshots"))
			}
			return strings.Join(append(parts, problems(o)), " · ")
		}},
		"scene.validate": {"edit.validate", func(a Args) string {
			return i18n.Tr("editor.agent.step.validate_scene", "Check {file}", i18n.Str("file", file(a)))
		}, problems},
		"schema.list": {"cat.engine", func(Args) string {
			return i18n.Tr("editor.agent.step.list_types", "List components and data types")
		}, func(o *Output) string {
			return i18n.TrN("editor.agent.step.types", int64(items(o)), "{n} type", "{n} types")
		}},
		"schema.show": {"cat.engine", func(a Args) string {
			return i18n.Tr("editor.agent.step.show_type", "Read the fields of {type}", i18n.Str("type", a.Str("type")))
		}, ok},
		"asset.list": {"panel.assets", func(Args) string {
			return i18n.Tr("editor.agent.step.list_assets", "List assets")
		}, count},
		"asset.move": {"edit.rename", func(a Args) string {
			return i18n.Tr("editor.agent.step.move_asset", "Move {asset} to {to}", i18n.Str("asset", path.Base(a.Str("asset"))), i18n.Str("to", a.Str("to")))
		}, changes},
		"asset.rename": {"edit.rename", func(a Args) string {
			return i18n.Tr("editor.agent.step.rename_asset", "Rename {asset} to {name}", i18n.Str("asset", path.Base(a.Str("asset"))), i18n.Str("name", a.Str("name")))
		}, changes},
		"asset.delete": {"edit.delete", func(a Args) string {
			return i18n.Tr("editor.agent.step.delete_asset", "Delete {asset}", i18n.Str("asset", path.Base(a.Str("asset"))))
		}, changes},
		"refs.find": {"panel.references", func(a Args) string {
			return i18n.Tr("editor.agent.step.find_refs", "Find what uses {asset}", i18n.Str("asset", path.Base(a.Str("asset"))))
		}, count},
		"refs.missing": {"edit.validate", func(Args) string {
			return i18n.Tr("editor.agent.step.missing_refs", "Look for missing references")
		}, count},
		"refs.fix": {"panel.references", func(a Args) string {
			return i18n.Tr("editor.agent.step.fix_refs", "Point references at {to}", i18n.Str("to", path.Base(a.Str("to"))))
		}, changes},
		"project.get": {"doc.project", func(Args) string {
			return i18n.Tr("editor.agent.step.read_project", "Read project settings")
		}, ok},
		"project.set": {"doc.project", func(a Args) string {
			return i18n.Tr("editor.agent.step.set_project", "Set {target}", i18n.Str("target", a.Str("section")+" › "+a.Str("field")))
		}, changes},
		"project.validate": {"edit.validate", func(Args) string {
			return i18n.Tr("editor.agent.step.validate_project", "Check the project")
		}, problems},
		"prefab.create": {"file.prefab", func(a Args) string {
			return i18n.Tr("editor.agent.step.make_prefab", "Make a prefab of {node}", i18n.Str("node", node(a)))
		}, changes},
		"prefab.instantiate": {"edit.add", func(a Args) string {
			return i18n.Tr("editor.agent.step.instantiate", "Add an instance of {prefab}", i18n.Str("prefab", path.Base(a.Str("prefab"))))
		}, changes},
		"prefab.apply": {"prefab.apply", func(a Args) string {
			return i18n.Tr("editor.agent.step.apply_overrides", "Apply the overrides of {node}", i18n.Str("node", node(a)))
		}, changes},
		"prefab.revert": {"prefab.revert", func(a Args) string {
			return i18n.Tr("editor.agent.step.revert_overrides", "Revert the overrides of {node}", i18n.Str("node", node(a)))
		}, changes},
		"prefab.unpack": {"prefab.unpack", func(a Args) string {
			return i18n.Tr("editor.agent.step.unpack", "Unpack {node}", i18n.Str("node", node(a)))
		}, changes},
		"prefab.overrides": {"file.prefab", func(a Args) string {
			return i18n.Tr("editor.agent.step.overrides", "List the overrides of {node}", i18n.Str("node", node(a)))
		}, count},
		"data.get": {"file.data", func(a Args) string {
			return i18n.Tr("editor.agent.step.read_data", "Read {file}", i18n.Str("file", file(a)))
		}, ok},
		"data.set": {"edit.set", func(a Args) string {
			return i18n.Tr("editor.agent.step.set_data", "Set {target}", i18n.Str("target", file(a)+" › "+a.Str("path")))
		}, changes},
		"data.reset": {"edit.set", func(a Args) string {
			return i18n.Tr("editor.agent.step.reset_data", "Reset {target}", i18n.Str("target", file(a)+" › "+a.Str("path")))
		}, changes},
		"data.base": {"edit.set", func(a Args) string {
			return i18n.Tr("editor.agent.step.base", "Set the base of {file}", i18n.Str("file", file(a)))
		}, changes},
		"data.rows": {"file.table", func(a Args) string {
			return i18n.Tr("editor.agent.step.rows", "List the rows of {file}", i18n.Str("file", file(a)))
		}, func(o *Output) string {
			return i18n.TrN("editor.agent.step.rows_n", int64(items(o)), "{n} row", "{n} rows")
		}},
		"data.add-row": {"edit.add", func(a Args) string {
			return i18n.Tr("editor.agent.step.add_row", "Add row {id} to {file}", i18n.Str("id", a.Str("id")), i18n.Str("file", file(a)))
		}, changes},
		"data.remove-row": {"edit.remove", func(a Args) string {
			return i18n.Tr("editor.agent.step.remove_row", "Remove row {id} from {file}", i18n.Str("id", a.Str("id")), i18n.Str("file", file(a)))
		}, changes},
		"data.rename-row": {"edit.rename", func(a Args) string {
			return i18n.Tr("editor.agent.step.rename_row", "Rename row {id} to {name}", i18n.Str("id", a.Str("id")), i18n.Str("name", a.Str("name")))
		}, changes},
		"data.import": {"file.table", func(a Args) string {
			return i18n.Tr("editor.agent.step.import", "Import {csv} into {file}", i18n.Str("csv", path.Base(a.Str("csv"))), i18n.Str("file", file(a)))
		}, changes},
		"data.export": {"file.download", func(a Args) string {
			return i18n.Tr("editor.agent.step.export", "Export {file} to {csv}", i18n.Str("file", file(a)), i18n.Str("csv", path.Base(a.Str("csv"))))
		}, ok},
		"anim.get": {"anim.key", func(a Args) string {
			return i18n.Tr("editor.agent.step.read_clip", "Read clip {file}", i18n.Str("file", file(a)))
		}, ok},
		"anim.set": {"edit.set", func(a Args) string {
			return i18n.Tr("editor.agent.step.set_clip", "Set {attr} of {file}", i18n.Str("attr", a.Str("attr")), i18n.Str("file", file(a)))
		}, changes},
		"anim.set-key": {"anim.key", func(a Args) string {
			return i18n.Tr("editor.agent.step.set_key", "Set a key of {target} at {time} s", i18n.Str("target", a.Str("target")+"."+a.Str("prop")), i18n.Str("time", a.Str("time")))
		}, changes},
		"anim.remove-key": {"edit.remove", func(a Args) string {
			return i18n.Tr("editor.agent.step.remove_key", "Remove the key of {target} at {time} s", i18n.Str("target", a.Str("target")+"."+a.Str("prop")), i18n.Str("time", a.Str("time")))
		}, changes},
		"anim.move-key": {"anim.key", func(a Args) string {
			return i18n.Tr("editor.agent.step.move_key", "Move a key of {target} to {to} s", i18n.Str("target", a.Str("target")+"."+a.Str("prop")), i18n.Str("to", a.Str("to")))
		}, changes},
		"anim.remove-track": {"edit.remove", func(a Args) string {
			return i18n.Tr("editor.agent.step.remove_track", "Remove track {target}", i18n.Str("target", a.Str("target")+"."+a.Str("prop")))
		}, changes},
		"anim.add-event": {"anim.event", func(a Args) string {
			return i18n.Tr("editor.agent.step.add_event", "Add event {name} at {time} s", i18n.Str("name", a.Str("name")), i18n.Str("time", a.Str("time")))
		}, changes},
		"anim.remove-event": {"edit.remove", func(a Args) string {
			return i18n.Tr("editor.agent.step.remove_event", "Remove an event from {file}", i18n.Str("file", file(a)))
		}, changes},
		"anim.move-event": {"anim.event", func(a Args) string {
			return i18n.Tr("editor.agent.step.move_event", "Move an event to {time} s", i18n.Str("time", a.Str("time")))
		}, changes},
		"anim.expand-frames": {"anim.slice", func(a Args) string {
			return i18n.Tr("editor.agent.step.expand", "Turn the frames of {file} into keys", i18n.Str("file", file(a)))
		}, changes},
		"ctrl.get": {"anim.graph", func(a Args) string {
			return i18n.Tr("editor.agent.step.read_ctrl", "Read controller {file}", i18n.Str("file", file(a)))
		}, ok},
		"ctrl.add-param": {"edit.add", func(a Args) string {
			return i18n.Tr("editor.agent.step.add_param", "Add parameter {name}", i18n.Str("name", a.Str("name")))
		}, changes},
		"ctrl.remove-param": {"edit.remove", func(a Args) string {
			return i18n.Tr("editor.agent.step.remove_param", "Remove parameter {name}", i18n.Str("name", a.Str("name")))
		}, changes},
		"ctrl.add-state": {"edit.add", func(a Args) string {
			return i18n.Tr("editor.agent.step.add_state", "Add state {name}", i18n.Str("name", a.Str("name")))
		}, changes},
		"ctrl.set-state": {"edit.set", func(a Args) string {
			return i18n.Tr("editor.agent.step.set_state", "Set {attr} of state {name}", i18n.Str("attr", a.Str("attr")), i18n.Str("name", a.Str("name")))
		}, changes},
		"ctrl.remove-state": {"edit.remove", func(a Args) string {
			return i18n.Tr("editor.agent.step.remove_state", "Remove state {name}", i18n.Str("name", a.Str("name")))
		}, changes},
		"ctrl.add-transition": {"anim.graph", func(a Args) string {
			return i18n.Tr("editor.agent.step.add_transition", "Add a transition {from} → {to}", i18n.Str("from", a.Str("from")), i18n.Str("to", a.Str("to")))
		}, changes},
		"ctrl.set-transition": {"edit.set", func(a Args) string {
			return i18n.Tr("editor.agent.step.set_transition", "Set {attr} of a transition", i18n.Str("attr", a.Str("attr")))
		}, changes},
		"ctrl.remove-transition": {"edit.remove", func(a Args) string {
			return i18n.Tr("editor.agent.step.remove_transition", "Remove a transition")
		}, changes},
		"ctrl.entry": {"anim.graph", func(a Args) string {
			return i18n.Tr("editor.agent.step.entry", "Start in state {name}", i18n.Str("name", a.Str("name")))
		}, changes},
		"script.read": {"file.script", func(a Args) string {
			return i18n.Tr("editor.agent.step.read_script", "Read script {file}", i18n.Str("file", file(a)))
		}, func(o *Output) string {
			return i18n.TrN("editor.agent.step.lines", int64(lines(o)), "{n} line", "{n} lines")
		}},
		"script.edit": {"edit.set", func(a Args) string {
			return i18n.Tr("editor.agent.step.edit_script", "Edit {file}", i18n.Str("file", file(a)))
		}, func(o *Output) string {
			if len(o.Changes) == 1 {
				return o.Changes[0].Path
			}
			return ""
		}},
		"script.write": {"edit.set", func(a Args) string {
			return i18n.Tr("editor.agent.step.write_script", "Rewrite {file}", i18n.Str("file", file(a)))
		}, changes},
		"script.create": {"file.script", func(a Args) string {
			return i18n.Tr("editor.agent.step.new_script", "Create script {file}", i18n.Str("file", file(a)))
		}, func(o *Output) string {
			if len(o.Changes) == 1 {
				return o.Changes[0].Path
			}
			return ""
		}},
	}
}

// extraLooks are the tools the MCP server and the editor add next to the commands.
var extraLooks = map[string]Look{
	"docs_search": {"doc.search", func(a Args) string {
		return i18n.Tr("editor.agent.step.search_docs", "Search the docs for “{query}”", i18n.Str("query", a.Str("query")))
	}, func(o *Output) string {
		return i18n.TrN("editor.agent.step.results", int64(items(o)), "{n} result", "{n} results")
	}},
	"docs_read": {"doc.page", func(a Args) string {
		return i18n.Tr("editor.agent.step.read_docs", "Read docs page {id}", i18n.Str("id", a.Str("id")))
	}, func(*Output) string { return "" }},
	"ask_user": {"agent.avatar", func(a Args) string {
		return i18n.Tr("editor.agent.step.ask", "Asked you: {question}", i18n.Str("question", clip(a.Str("question"), 60)))
	}, func(*Output) string { return "" }},
}
