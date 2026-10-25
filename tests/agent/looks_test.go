package agent_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"aego/app/agent"
	"aego/scene/mutate"
)

// Every tool has an icon from the icon set, a title without JSON and a short summary, so the Agent panel never shows a raw call (11b5a).
func TestEveryToolHasALook(t *testing.T) {
	tools := []string{"docs_search", "docs_read", "ask_user"}
	for _, c := range agent.Commands() {
		tools = append(tools, c.Tool())
	}
	args := agent.Args{"file": "scenes/main.aescene", "node": "Player/Sprite", "path": "transform.position", "name": "Ledge", "comp": "SpriteRenderer",
		"asset": "sprites/hero.png", "to": "sprites/new.png", "query": "load next scene", "id": "save/load", "type": "gameplay.Health",
		"section": "physics", "field": "gravity", "prefab": "prefabs/enemy.aeprefab", "parent": "World", "csv": "data/items.csv",
		"target": "Sprite", "prop": "frame", "time": 0.5, "attr": "loop", "from": "Idle", "tx": `tx version=1 { label="Add Ledge" }`, "question": "Which level?"}
	value, _ := json.Marshal([]string{"a", "b"})
	out := &agent.Output{Value: json.RawMessage(value), Changes: []mutate.Change{{Op: "set", Node: "Root/Player", Path: "transform.position", Before: "[0, 0]", After: "[1, 2]"}}}
	for _, tool := range tools {
		l, ok := agent.Present(tool)
		switch {
		case !ok:
			t.Errorf("%s has no look", tool)
			continue
		case l.Icon == "" || l.Title == nil || l.Summary == nil:
			t.Errorf("%s: the look needs Icon, Title and Summary", tool)
			continue
		}
		if _, err := os.Stat(filepath.Join("..", "..", "engine", "res", "icons", "src", l.Icon+".svg")); err != nil {
			t.Errorf("%s: icon %s is not in the icon set", tool, l.Icon)
		}
		if title := l.Title(args); title == "" || strings.ContainsAny(title, "{}") {
			t.Errorf("%s: title %q is empty or holds JSON or a placeholder", tool, title)
		}
		if s := l.Summary(out); utf8.RuneCountInString(s) > 80 || strings.Contains(s, "{") {
			t.Errorf("%s: summary %q is longer than 80 characters or holds JSON", tool, s)
		}
	}
}
