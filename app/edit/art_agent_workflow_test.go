package edit

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"aego/app/agent"
	"aego/app/agent/mcp"
	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
	"aego/art"
	"aego/editor/core/artedit"
	"aego/editor/shell"
)

func TestArtAgentUsesLiveDocumentApprovalAndHistory(t *testing.T) {
	p := newProject(t)
	drawing, err := art.New(8, 8, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	source, err := drawing.Encode()
	drawing.Close()
	if err != nil {
		t.Fatal(err)
	}
	const path = "assets/agent-art.aeart"
	if err := os.WriteFile(filepath.Join(p.Root, path), source, 0644); err != nil {
		t.Fatal(err)
	}
	cfg := headless()
	cfg.AgentBackend = &backend.Fake{}
	s, err := Open(p, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ed := s.App.Context()
	doc, err := ed.OpenAssetPath(path)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok {
		t.Fatal("art view has no shared model")
	}
	bridge, err := s.agentBridge()
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		out *agent.Output
		err error
	}
	call := func(name string, args agent.Args, decide func(*agentbridge.Proposal)) result {
		t.Helper()
		finished := make(chan result, 1)
		go func() {
			out, err := bridge.Run(name, args)
			finished <- result{out, err}
		}()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			if !s.Step() {
				t.Fatal("editor stopped during agent call")
			}
			if decide != nil {
				for _, pending := range bridge.Pending() {
					decide(pending)
				}
			}
			select {
			case r := <-finished:
				return r
			default:
			}
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("agent call %s timed out", name)
		return result{}
	}
	paint := func(point image.Point) {
		t.Helper()
		_, err := model.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{point}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	paint(image.Pt(2, 2))
	capture := call("art.capture", agent.Args{"file": path}, nil)
	if capture.err != nil || capture.out == nil || len(capture.out.Images) != 1 {
		t.Fatalf("agent capture: %+v", capture)
	}
	img, err := png.Decode(bytes.NewReader(capture.out.Images[0].Data))
	if err != nil {
		t.Fatal(err)
	}
	r, _, _, a := img.At(2, 2).RGBA()
	if r != 0xffff || a != 0xffff {
		t.Fatal("agent captured disk pixels instead of the unsaved drawing")
	}
	readRevision := func() string {
		t.Helper()
		read := call("art.read", agent.Args{"file": path}, nil)
		if read.err != nil || read.out == nil {
			t.Fatalf("agent read: %+v", read)
		}
		value, ok := read.out.Value.(map[string]any)
		if !ok || value["dirty"] != true {
			t.Fatalf("agent read lost the unsaved document: %+v", read.out.Value)
		}
		revision, ok := value["revision"].(string)
		if !ok || revision == "" {
			t.Fatal("agent did not receive a revision")
		}
		return revision
	}
	args := func(revision string) agent.Args {
		operations := make([]art.Operation, 7)
		for i := range operations {
			operations[i] = art.Operation{Kind: "add-frame", Frame: model.Session.Frame, DurationMS: 90 + i*10}
		}
		return agent.Args{"file": path, "revision": revision, "label": "Create eight-frame animation", "operations": operations}
	}
	approved := false
	result1 := call("art.apply", args(readRevision()), func(p *agentbridge.Proposal) {
		if len(model.Session.Document.Info().Frames) != 1 || model.History.Cursor() != 1 {
			t.Fatal("proposal mutated the document before approval")
		}
		approved = bridge.Decide(p.ID, true)
	})
	if !approved || result1.err != nil || len(model.Session.Document.Info().Frames) != 8 || model.History.Cursor() != 2 {
		t.Fatalf("approved proposal did not use the common history: %+v", result1)
	}
	if err := model.Session.Select("", model.Session.Document.Info().Frames[4].ID, art.Albedo); err != nil {
		t.Fatal(err)
	}
	paint(image.Pt(3, 3))
	corrected := call("art.capture", agent.Args{"file": path}, nil)
	if corrected.err != nil || corrected.out == nil || len(corrected.out.Images) != 1 {
		t.Fatalf("capture after user correction: %+v", corrected)
	}
	correctedPixels, err := png.Decode(bytes.NewReader(corrected.out.Images[0].Data))
	if err != nil {
		t.Fatal(err)
	}
	r, _, _, a = correctedPixels.At(3, 3).RGBA()
	if r != 0xffff || a != 0xffff || corrected.out.Value.(map[string]any)["frame"] != model.Session.Frame {
		t.Fatal("agent did not read the user's current unsaved fifth frame")
	}
	if err := ed.UndoAsset(doc); err != nil {
		t.Fatal(err)
	}
	if err := ed.UndoAsset(doc); err != nil || len(model.Session.Document.Info().Frames) != 1 || model.History.Cursor() != 1 {
		t.Fatalf("shared Undo did not reverse the agent batch as one step: %v", err)
	}
	stale := call("art.apply", args(readRevision()), func(p *agentbridge.Proposal) {
		paint(image.Pt(4, 4))
		if !bridge.Decide(p.ID, true) {
			t.Fatal("could not approve stale proposal")
		}
	})
	if stale.err == nil || len(model.Session.Document.Info().Frames) != 1 || model.History.Cursor() != 2 {
		t.Fatal("stale proposal overwrote a later user edit")
	}
	rejected := call("art.apply", args(readRevision()), func(p *agentbridge.Proposal) {
		if !bridge.Decide(p.ID, false) {
			t.Fatal("could not reject proposal")
		}
	})
	if rejected.err == nil || len(model.Session.Document.Info().Frames) != 1 || model.History.Cursor() != 2 {
		t.Fatal("rejected proposal changed the document")
	}
	addr, token, ok := agent.EditorProxy(p.Root)
	if !ok {
		t.Fatal("editor did not expose its local CLI/MCP proxy")
	}
	if err := ed.Commands.Run(shell.CommandProxyConfirm); err != nil {
		t.Fatal(err)
	}
	type externalResult struct {
		code   int
		output string
		err    error
	}
	external := func(run func() externalResult, decide func(*agentbridge.Proposal)) externalResult {
		t.Helper()
		finished := make(chan externalResult, 1)
		go func() { finished <- run() }()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			if !s.Step() {
				t.Fatal("editor stopped during external tool call")
			}
			if decide != nil {
				for _, pending := range bridge.Pending() {
					decide(pending)
				}
			}
			select {
			case r := <-finished:
				return r
			default:
			}
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		}
		t.Fatal("external art call timed out")
		return externalResult{}
	}
	outputPath := filepath.Join(t.TempDir(), "unsaved.png")
	captureCLI := func() externalResult {
		var out, errOut bytes.Buffer
		code := agent.Main(p, []string{"art", "capture", path, "--out", outputPath, "--json"}, &out, &errOut, agent.Options{})
		return externalResult{code: code, output: out.String() + errOut.String()}
	}
	cliCapture := external(captureCLI, nil)
	if cliCapture.code != 0 {
		t.Fatalf("CLI capture failed: %s", cliCapture.output)
	}
	encoded, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	img, err = png.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	r, _, _, a = img.At(2, 2).RGBA()
	if r != 0xffff || a != 0xffff {
		t.Fatal("CLI capture did not write the unsaved pixels")
	}
	if again := external(captureCLI, nil); again.code == 0 {
		t.Fatal("CLI capture overwrote an existing file")
	}
	if current, err := os.ReadFile(outputPath); err != nil || !bytes.Equal(current, encoded) {
		t.Fatal("refused CLI capture changed the existing file")
	}
	revision := readRevision()
	operations := fmt.Sprintf(`[{"kind":"add-frame","frame":%q,"durationMS":120}]`, model.Session.Frame)
	cli := external(func() externalResult {
		var out, errOut bytes.Buffer
		code := agent.Main(p, []string{"art", "apply", path, "--revision", revision, "--label", "CLI frame", "--operations", operations, "--json"}, &out, &errOut, agent.Options{})
		return externalResult{code: code, output: out.String() + errOut.String()}
	}, func(p *agentbridge.Proposal) {
		if p.Via != "via CLI" || len(model.Session.Document.Info().Frames) != 1 {
			t.Fatal("CLI bypassed the configured confirmation or changed the document")
		}
		bridge.Decide(p.ID, true)
	})
	if cli.code != 0 || len(model.Session.Document.Info().Frames) != 2 {
		t.Fatalf("CLI art apply failed: %s", cli.output)
	}
	labels, _ := model.History.Labels(1)
	if len(labels) != 1 || !strings.HasPrefix(labels[0], "via CLI: ") {
		t.Fatalf("CLI art history lost its source: %v", labels)
	}
	request := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"art-test","version":"1"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"art_capture","arguments":{"file":"` + path + `"}}}` + "\n")
	mcpResult := external(func() externalResult {
		var out bytes.Buffer
		err := mcp.Relay(request, &out, addr, token)
		return externalResult{output: out.String(), err: err}
	}, nil)
	if mcpResult.err != nil {
		t.Fatal(mcpResult.err)
	}
	var captured bool
	for _, line := range strings.Split(strings.TrimSpace(mcpResult.output), "\n") {
		var reply struct {
			ID     int `json:"id"`
			Result struct {
				IsError bool `json:"isError"`
				Content []struct {
					Type, MimeType, Data string
				} `json:"content"`
			} `json:"result"`
			Error any `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &reply); err != nil || reply.Error != nil || reply.Result.IsError {
			t.Fatalf("MCP response: %s (%v)", line, err)
		}
		if reply.ID == 2 {
			for _, content := range reply.Result.Content {
				if content.Type != "image" || content.MimeType != "image/png" {
					continue
				}
				data, err := base64.StdEncoding.DecodeString(content.Data)
				if err != nil {
					t.Fatal(err)
				}
				image, err := png.Decode(bytes.NewReader(data))
				if err != nil {
					t.Fatal(err)
				}
				r, _, _, a := image.At(2, 2).RGBA()
				captured = r == 0xffff && a == 0xffff
			}
		}
	}
	if !captured {
		t.Fatal("MCP did not receive a PNG capture of the live art document")
	}
	revision = readRevision()
	mcpInput := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"art-test","version":"1"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"art_apply","arguments":{"file":%q,"revision":%q,"label":"MCP frame","operations":%s}}}`, path, revision, operations) + "\n")
	mcpApply := external(func() externalResult {
		var out bytes.Buffer
		err := mcp.Relay(mcpInput, &out, addr, token)
		return externalResult{output: out.String(), err: err}
	}, func(p *agentbridge.Proposal) {
		if p.Via != "via agent: art-test" || len(model.Session.Document.Info().Frames) != 2 {
			t.Fatal("MCP bypassed configured approval or modified the document")
		}
		bridge.Decide(p.ID, true)
	})
	if mcpApply.err != nil || len(model.Session.Document.Info().Frames) != 3 {
		t.Fatalf("MCP art apply failed: %s (%v)", mcpApply.output, mcpApply.err)
	}
	for _, line := range strings.Split(strings.TrimSpace(mcpApply.output), "\n") {
		var reply struct {
			Result struct {
				IsError bool `json:"isError"`
			} `json:"result"`
			Error any `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &reply); err != nil || reply.Error != nil || reply.Result.IsError {
			t.Fatalf("MCP apply response: %s (%v)", line, err)
		}
	}
	labels, _ = model.History.Labels(1)
	if len(labels) != 1 || !strings.HasPrefix(labels[0], "via agent: art-test: ") {
		t.Fatalf("MCP art history lost its source: %v", labels)
	}
	if err := ed.UndoAsset(doc); err != nil || len(model.Session.Document.Info().Frames) != 2 {
		t.Fatalf("MCP edit was not one shared Undo step: %v", err)
	}
	if disk, err := os.ReadFile(ed.Docs.Abs(path)); err != nil || !bytes.Equal(disk, source) {
		t.Fatal("agent workflow saved the drawing without user Save")
	}
}
