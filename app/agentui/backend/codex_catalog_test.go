package backend

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEditorCatalogRejectsInvalidOrMismatchedTools(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		ok   bool
	}{
		{"named", `{"name":"scene_tree"}`, true},
		{"key-only", `{}`, true},
		{"null", `null`, false},
		{"mismatch", `{"name":"different"}`, false},
		{"invalid-type", `"scene_tree"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyEditorCatalog(map[string]json.RawMessage{"scene_tree": json.RawMessage(tc.raw)}, []string{"scene_tree"})
			if (err == nil) != tc.ok || err != nil && !strings.Contains(err.Error(), "scene_tree") {
				t.Fatalf("catalog verification: %v", err)
			}
		})
	}
}
