package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// The CLI's bundled catalog may require an executor even for an editor-only
// MCP call. For the selected model expose ordinary function tools instead.
// The catalog is private to this child process and removed when it exits.
func editorToolCatalog(parent context.Context, cfg Config) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, codexBinary(cfg), "debug", "models", "--bundled").Output()
	if err != nil {
		return "", nil
	}
	var catalog map[string]json.RawMessage
	var models []map[string]json.RawMessage
	if json.Unmarshal(raw, &catalog) != nil || json.Unmarshal(catalog["models"], &models) != nil {
		return "", nil
	}
	changed := false
	for _, m := range models {
		var slug, mode string
		if json.Unmarshal(m["slug"], &slug) != nil || (cfg.Model != "" && slug != cfg.Model) || json.Unmarshal(m["tool_mode"], &mode) != nil || mode != "code_mode_only" {
			continue
		}
		// Leave the model ID, instructions, reasoning options and image capabilities intact.
		delete(m, "tool_mode")
		m["supports_search_tool"] = json.RawMessage("false")
		m["apply_patch_tool_type"] = json.RawMessage("null")
		m["experimental_supported_tools"] = json.RawMessage("[]")
		delete(m, "multi_agent_version")
		changed = true
	}
	if !changed {
		return "", nil
	}
	catalog["models"], err = json.Marshal(models)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "aego-editor-tools-*.json")
	if err != nil {
		return "", err
	}
	err = json.NewEncoder(f).Encode(catalog)
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("write editor tool catalog: %w", err)
	}
	return f.Name(), nil
}
