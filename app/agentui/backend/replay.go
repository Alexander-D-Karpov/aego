package backend

import (
	"bytes"
	"encoding/json"
)

// Replay plays a recorded Claude Code stream-json session, one turn per Send: its text and tool calls go through the same decoder as a live session, and the calls run against this editor's MCP server, so the results are this project's (11b5b).
type Replay struct {
	// Turns are the stream-json lines of each turn, as claude -p --output-format stream-json writes them.
	Turns [][]byte
}

func (r *Replay) Name() string {
	return "replay"
}

func (r *Replay) Available(Config) (string, error) {
	return "replay", nil
}

// ReplayTurns splits a recorded session into turns at its result lines.
func ReplayTurns(session []byte) [][]byte {
	var out [][]byte
	var cur bytes.Buffer
	for _, line := range bytes.Split(session, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		cur.Write(line)
		cur.WriteByte('\n')
		var head struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(line, &head) == nil && head.Type == "result" {
			out = append(out, append([]byte(nil), cur.Bytes()...))
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.Bytes())
	}
	return out
}

func (r *Replay) Start(cfg Config) (Conversation, error) {
	var turns [][]Step
	for _, t := range r.Turns {
		turns = append(turns, replaySteps(t))
	}
	return (&Fake{Turns: turns}).Start(cfg)
}

// replaySteps decodes the lines of a turn and keeps its text and tool calls; the recorded results are dropped, the calls run again.
func replaySteps(turn []byte) []Step {
	var steps []Step
	for _, line := range bytes.Split(turn, []byte("\n")) {
		for _, e := range Decode(line) {
			switch e.Kind {
			case EvText:
				steps = append(steps, Step{Text: e.Text})
			case EvToolStart:
				var args map[string]any
				_ = json.Unmarshal(e.Args, &args)
				steps = append(steps, Step{Tool: e.Tool, Args: args})
			}
		}
	}
	return steps
}
