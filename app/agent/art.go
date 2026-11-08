package agent

import (
	"fmt"
	"strconv"
	"strings"

	"aego/art"
)

// ArtHost is supplied only by the running editor. Offline commands must not
// reopen files and silently replace the user's dirty graphical documents.
// Like Host.ApplyScene, these calls run on the editor mutation thread.
type ArtHost interface {
	ReadArt(file, section string, offset, limit int) (*Output, error)
	CaptureArt(file string, frame art.ID, channel art.Channel) (*Output, error)
	PrepareArt(file string, revision uint64, label string, ops []art.Operation) (*Output, error)
}

// ArtProposal is an opaque, prepared transaction carried by the existing agent
// proposal queue. Commit is invoked only after its normal approval decision.
// Keeping the prepared transaction also preserves IDs returned by its preview.
type ArtProposal interface {
	Commit() (*Output, error)
	RetainedBytes() int64
}

func artHost(s *Session) (ArtHost, error) {
	if s != nil {
		if h, ok := s.host.(ArtHost); ok {
			return h, nil
		}
	}
	return nil, fmt.Errorf("art: connect to the running Aego editor; offline writes to graphical documents are disabled")
}
func artCommands() []*Command {
	fileArg := Arg{Name: "file", Type: "string", Doc: "Project-relative .aeart source, including unsaved editor content", Required: true, Pos: true}
	return []*Command{
		{Name: "art.read", Doc: "Read the current art document, not its last saved file. Lists are paginated; revision is a decimal string for art.apply.", Args: []Arg{
			fileArg, {Name: "section", Type: "string", Doc: "summary, layers, frames, cels, palette, tags or slices"},
			{Name: "offset", Type: "integer", Doc: "First item, default 0"}, {Name: "limit", Type: "integer", Doc: "1..1024 items; default 256"},
		}, Run: func(s *Session, a Args) (*Output, error) {
			h, err := artHost(s)
			if err != nil {
				return nil, err
			}
			offset, err := a.Int("offset", 0)
			if err != nil {
				return nil, err
			}
			limit, err := a.Int("limit", 256)
			if err != nil {
				return nil, err
			}
			if offset < 0 || limit < 1 || limit > 1024 {
				return nil, fmt.Errorf("art: invalid page")
			}
			return h.ReadArt(a.Str("file"), a.Str("section"), offset, limit)
		}},
		{Name: "art.capture", Doc: "Capture unsaved CPU pixels with document/frame/channel identity and revision. MCP returns a PNG image; the CLI writes it only with --out to a new file.", Args: []Arg{
			fileArg, {Name: "frame", Type: "string", Doc: "Stable frame ID; defaults to current frame"},
			{Name: "channel", Type: "string", Doc: "albedo, normal, height or emission; defaults to current channel"},
		}, Run: func(s *Session, a Args) (*Output, error) {
			h, err := artHost(s)
			if err != nil {
				return nil, err
			}
			return h.CaptureArt(a.Str("file"), art.ID(a.Str("frame")), art.Channel(a.Str("channel")))
		}},
		{Name: "art.apply", Doc: "Propose one atomic graphics edit at the revision returned by art.read. Agent panel edits require approval; CLI and external agents follow the editor's confirmation setting. All use the document's common Undo. Name new entities with as and refer to them as $name later.", Write: true, Destroys: true, Args: []Arg{
			fileArg, {Name: "revision", Type: "string", Doc: "Exact decimal revision returned by art.read", Required: true},
			{Name: "label", Type: "string", Doc: "Human-readable operation label", Required: true},
			{Name: "operations", Type: "array", Doc: "1..4096 typed art.Operation objects; no paths or shell commands", Required: true, Items: map[string]any{"type": "object"}},
		}, Run: func(s *Session, a Args) (*Output, error) {
			h, err := artHost(s)
			if err != nil {
				return nil, err
			}
			if !a.Bool("dry-run") {
				return nil, fmt.Errorf("art: edits must pass through the editor proposal queue")
			}
			revision, err := strconv.ParseUint(a.Str("revision"), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("art: revision must be an unsigned decimal integer")
			}
			label := strings.TrimSpace(a.Str("label"))
			if label == "" || len(label) > 256 {
				return nil, fmt.Errorf("art: label must have 1..256 bytes")
			}
			ops, err := art.DecodeOperations([]byte(a.Str("operations")))
			if err != nil {
				return nil, err
			}
			return h.PrepareArt(a.Str("file"), revision, label, ops)
		}},
	}
}
