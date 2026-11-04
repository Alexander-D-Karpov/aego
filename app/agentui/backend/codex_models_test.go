package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

func init() {
	mode := os.Getenv("AEGO_CODEX_MODELS_FIXTURE")
	if mode == "" {
		return
	}
	if mode == "hang" {
		time.Sleep(time.Hour)
		os.Exit(0)
	}
	enc, dec := json.NewEncoder(os.Stdout), json.NewDecoder(os.Stdin)
	for {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Cursor string `json:"cursor"`
			} `json:"params"`
		}
		if dec.Decode(&req) != nil {
			os.Exit(0)
		}
		switch req.Method {
		case "initialize":
			_ = enc.Encode(map[string]any{"id": req.ID, "result": map[string]any{}})
		case "initialized":
		case "model/list":
			_ = enc.Encode(map[string]any{"method": "notice", "params": map[string]any{}})
			if mode == "error" {
				_ = enc.Encode(map[string]any{"id": req.ID, "error": map[string]any{"code": -1, "message": "no catalog"}})
				continue
			}
			if req.Params.Cursor == "" {
				_ = enc.Encode(map[string]any{"id": req.ID, "result": map[string]any{"nextCursor": "page2", "data": []any{
					map[string]any{"id": "first", "model": "model-one", "displayName": "Model One", "description": "First model", "supportedReasoningEfforts": []any{map[string]string{"reasoningEffort": "low"}, map[string]string{"reasoningEffort": "high"}}},
					map[string]any{"model": "hidden", "hidden": true},
				}}})
			} else {
				_ = enc.Encode(map[string]any{"id": req.ID, "result": map[string]any{"nextCursor": nil, "data": []any{
					map[string]any{"model": "model-one"}, map[string]any{"id": "model-two", "displayName": "Model Two"},
				}}})
			}
		default:
			fmt.Fprintln(os.Stderr, "unexpected request:", req.Method)
			os.Exit(2)
		}
	}
}

func TestCodexModelsPaginationAndErrors(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AEGO_CODEX_MODELS_FIXTURE", "catalog")
	models, err := CodexModels(context.Background(), exe, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "model-one" || models[0].Label != "Model One" || len(models[0].Efforts) != 2 || models[0].Efforts[1] != "high" || models[1].ID != "model-two" {
		t.Fatalf("unexpected catalog: %#v", models)
	}
	t.Setenv("AEGO_CODEX_MODELS_FIXTURE", "error")
	if _, err := CodexModels(context.Background(), exe, ""); err == nil {
		t.Fatal("server error was ignored")
	}
}

func TestCodexModelsTimeout(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AEGO_CODEX_MODELS_FIXTURE", "hang")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := codexModels(ctx, exe, ""); err == nil {
		t.Fatal("hanging catalog succeeded")
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("catalog did not terminate promptly: %v", elapsed)
	}
}

func TestCodexModelsRealCLI(t *testing.T) {
	command := os.Getenv("AEGO_CODEX_PROTOCOL_CLI")
	if command == "" {
		t.Skip("set AEGO_CODEX_PROTOCOL_CLI to test the installed CLI")
	}
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("CODEX_API_KEY", "")
	models, err := CodexModels(context.Background(), command, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) == 0 || models[0].ID == "" {
		t.Fatalf("CLI returned an empty catalog: %#v", models)
	}
}
