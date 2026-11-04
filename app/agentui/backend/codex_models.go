package backend

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// CodexModel is an entry advertised by the installed Codex CLI.
type CodexModel struct {
	ID, Label, Description string
	Efforts                []string
}

// CodexModels asks the CLI for its current model catalog without starting a
// conversation or changing its configuration. It does not create an MCP bridge.
func CodexModels(parent context.Context, command, dir string) ([]CodexModel, error) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	return codexModels(ctx, command, dir)
}

func codexModels(parent context.Context, command, dir string) ([]CodexModel, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	cmd := exec.CommandContext(ctx, codexBinary(Config{Command: command}), codexArgs()...)
	cmd.Dir = dir
	hideCatalogProcess(cmd)
	// The npm launcher forwards interrupts to the actual CLI. A bounded wait
	// also handles launchers that ignore the signal or keep stdout open.
	cmd.Cancel = func() error {
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	cmd.WaitDelay = 2 * time.Second
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = &codexStderr{}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		_ = out.Close()
	}()
	defer func() {
		cancel()
		_ = in.Close()
		_ = cmd.Wait()
	}()
	encoder := json.NewEncoder(in)
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	request := func(id int, method string, params any, result any) error {
		if err := encoder.Encode(map[string]any{"id": id, "method": method, "params": params}); err != nil {
			return err
		}
		for scanner.Scan() {
			var response codexEnvelope
			if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
				return err
			}
			if string(response.ID) != strconv.Itoa(id) {
				continue // Notifications are not replies to this request.
			}
			if response.Error != nil {
				return fmt.Errorf("model catalog: %s", response.Error.Message)
			}
			return json.Unmarshal(response.Result, result)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		return errors.New("Codex closed the model catalog")
	}
	var initialized json.RawMessage
	if err := request(1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "aego_editor", "version": "1"}}, &initialized); err != nil {
		return nil, err
	}
	if err := encoder.Encode(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		return nil, err
	}
	var models []CodexModel
	seen := map[string]bool{}
	cursor := ""
	for page := 0; page < 16; page++ {
		params := map[string]any{"limit": 100, "includeHidden": false}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var result struct {
			Data []struct {
				Model       string `json:"model"`
				ID          string `json:"id"`
				Label       string `json:"displayName"`
				Description string `json:"description"`
				Hidden      bool   `json:"hidden"`
				Efforts     []struct {
					Value string `json:"reasoningEffort"`
				} `json:"supportedReasoningEfforts"`
			} `json:"data"`
			Next string `json:"nextCursor"`
		}
		if err := request(page+2, "model/list", params, &result); err != nil {
			return nil, err
		}
		for _, m := range result.Data {
			id := strings.TrimSpace(m.Model)
			if id == "" {
				id = strings.TrimSpace(m.ID)
			}
			if id == "" || m.Hidden || seen[id] {
				continue
			}
			seen[id] = true
			entry := CodexModel{ID: id, Label: m.Label, Description: m.Description}
			for _, effort := range m.Efforts {
				if effort.Value != "" {
					entry.Efforts = append(entry.Efforts, effort.Value)
				}
			}
			models = append(models, entry)
		}
		if result.Next == "" {
			return models, nil
		}
		if result.Next == cursor {
			return nil, errors.New("Codex model catalog repeated a page")
		}
		cursor = result.Next
	}
	return nil, errors.New("Codex model catalog has too many pages")
}
