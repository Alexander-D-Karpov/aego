package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"aego/project"
	"aego/scene/mutate"
	"aego/script/diag"
	"aego/script/workspace"
)

var argScript = Arg{Name: "file", Type: "string", Doc: "script file, relative to the project (scripts/enemy/enemy.aes)", Required: true, Pos: true}

func scriptCommands() []*Command {
	return []*Command{
		{Name: "script.read", Doc: "text of a script, as the editor has it, with unsaved changes; from and to read a part of a long one", Args: []Arg{argScript,
			{Name: "from", Type: "integer", Doc: "first line to read, 1-based"}, {Name: "to", Type: "integer", Doc: "last line to read"}}, Run: (*Session).scriptGet},
		{Name: "script.edit", Doc: "replace text in a script; old must match exactly once, so quote enough lines around the change", Write: true,
			Args: []Arg{argScript,
				{Name: "old", Type: "string", Doc: "exact text to replace, with its indentation", Required: true},
				{Name: "new", Type: "string", Doc: "text that replaces it", Required: true}}, Run: (*Session).scriptEdit},
		{Name: "script.write", Doc: "replace the whole text of a script; for a small change script.edit is safer", Write: true,
			Args: []Arg{argScript, {Name: "text", Type: "string", Doc: "the whole script, starting with its package clause", Required: true}}, Run: (*Session).scriptWrite},
		{Name: "script.create", Doc: "create a script file; it fails when the file exists, so change an existing script with script.edit", Write: true,
			Args: []Arg{argScript, {Name: "text", Type: "string", Doc: "the whole script, starting with its package clause", Required: true}}, Run: (*Session).scriptNew},
	}
}

func (s *Session) scriptRel(a Args) (string, error) {
	rel, err := RelIn(s.Project.Root, a.Str("file"))
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(rel, ".aes") {
		return "", errors.New(rel + " is not a script; script commands take .aes files")
	}
	return rel, nil
}

// scriptText is the open editor buffer of a script, or the file on disk.
func (s *Session) scriptText(rel string) (string, error) {
	if s.host != nil {
		if text, ok := s.host.ScriptText(rel); ok {
			return text, nil
		}
	}
	data, err := os.ReadFile(filepath.Join(s.Project.Root, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Session) scriptGet(a Args) (*Output, error) {
	rel, err := s.scriptRel(a)
	if err != nil {
		return nil, err
	}
	text, err := s.scriptText(rel)
	if err != nil {
		return nil, err
	}
	from, err := a.Int("from", 0)
	if err != nil {
		return nil, err
	}
	to, err := a.Int("to", 0)
	if err != nil {
		return nil, err
	}
	part, note := window(text, from, to, s.budget.ScriptLines)
	out := &Output{Value: part, Text: part}
	if note != "" {
		out.Notes = append(out.Notes, note)
	}
	return out, nil
}

// window is the lines from..to of a script, at most max of them; a script within max lines comes whole when no range is asked.
func window(text string, from, to, limit int) (string, string) {
	lines := strings.SplitAfter(text, "\n")
	total := len(lines)
	if from <= 0 && to <= 0 && (limit <= 0 || total <= limit) {
		return text, ""
	}
	from = max(from, 1)
	if to <= 0 || to > total {
		to = total
	}
	if limit > 0 && to-from+1 > limit {
		to = from + limit - 1
	}
	if from > total || from > to {
		return "", "the script has " + strconv.Itoa(total) + " lines"
	}
	note := "lines " + strconv.Itoa(from) + "-" + strconv.Itoa(to) + " of " + strconv.Itoa(total)
	if to < total || from > 1 {
		note += "; read others with from and to"
	}
	return strings.Join(lines[from-1:to], ""), note
}

// ScriptEdit is a script change: the text before and after, and the first changed line; New creates the file.
type ScriptEdit struct {
	Rel           string
	Before, After string
	Line          int
	New           bool
}

func (s *Session) scriptNew(a Args) (*Output, error) {
	rel, err := s.scriptRel(a)
	if err != nil {
		return nil, err
	}
	abs := filepath.Join(s.Project.Root, filepath.FromSlash(rel))
	if _, err := os.Stat(abs); err == nil {
		return nil, errors.New(rel + " exists; change it with script.edit")
	}
	text := a.Str("text")
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	edit := ScriptEdit{Rel: rel, After: text, Line: 1, New: true}
	out := &Output{Changes: []mutate.Change{{Op: "script-new", Node: rel, Path: strconv.Itoa(strings.Count(text, "\n")) + " lines", After: text}}, Files: []string{rel}, Problems: s.scriptProblems(rel, text)}
	if a.Bool("dry-run") {
		out.DryRun = true
		return out, nil
	}
	if s.host != nil {
		return out, s.host.ApplyScript(edit, "Create "+rel)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, err
	}
	return out, project.WriteAtomic(abs, []byte(text))
}

func (s *Session) scriptWrite(a Args) (*Output, error) {
	rel, err := s.scriptRel(a)
	if err != nil {
		return nil, err
	}
	text, err := s.scriptText(rel)
	if err != nil {
		return nil, err
	}
	repl := a.Str("text")
	if !strings.HasSuffix(repl, "\n") {
		repl += "\n"
	}
	if repl == text {
		return &Output{Notes: []string{rel + " already has this text"}}, nil
	}
	return s.replaceScript(a, rel, text, text, repl)
}

func (s *Session) scriptEdit(a Args) (*Output, error) {
	rel, err := s.scriptRel(a)
	if err != nil {
		return nil, err
	}
	text, err := s.scriptText(rel)
	if err != nil {
		return nil, err
	}
	old, repl := a.Str("old"), a.Str("new")
	if old == "" {
		return nil, errors.New("script.edit needs the text to replace in old")
	}
	switch n := strings.Count(text, old); {
	case n == 0:
		return nil, errors.New("old does not match " + rel + "; read it again with script.read and copy the lines exactly")
	case n > 1:
		return nil, errors.New("old matches " + rel + " " + strconv.Itoa(n) + " times; quote more lines around the change")
	}
	return s.replaceScript(a, rel, text, old, repl)
}

// replaceScript replaces the one place of old in text with repl and shows it as the lines that changed.
func (s *Session) replaceScript(a Args, rel, text, old, repl string) (*Output, error) {
	at := strings.Index(text, old)
	edit := ScriptEdit{Rel: rel, Before: text, After: text[:at] + repl + text[at+len(old):], Line: strings.Count(text[:at], "\n") + 1}
	out := &Output{Changes: []mutate.Change{{Op: "script-edit", Node: rel, Path: "line " + strconv.Itoa(edit.Line), Before: old, After: repl}}, Files: []string{rel}, Problems: s.scriptProblems(rel, edit.After)}
	if a.Bool("dry-run") {
		out.DryRun = true
		return out, nil
	}
	if s.host != nil {
		return out, s.host.ApplyScript(edit, "Edit "+rel)
	}
	if err := project.WriteAtomic(filepath.Join(s.Project.Root, filepath.FromSlash(rel)), []byte(edit.After)); err != nil {
		return nil, err
	}
	return out, nil
}

// checker is the script workspace the edits are compiled in: the kit's in the CLI, one of its own in the editor.
func (s *Session) checker() *workspace.Workspace {
	if s.kit != nil {
		return s.kit.ws
	}
	if s.check == nil {
		ws, err := workspace.Open(workspace.Config{Root: s.Project.Root, Module: s.Project.Module, Scripts: s.Project.ScriptsDir()})
		if err != nil {
			return nil
		}
		if s.methods != nil && s.methods.binds != nil {
			ws.SetBindings(s.methods.binds.Table())
		}
		s.check = ws
	}
	return s.check
}

// scriptProblems compiles the scripts with rel as text, as aego check does, and returns what the compiler says about that file.
func (s *Session) scriptProblems(rel, text string) []mutate.Problem {
	ws := s.checker()
	if ws == nil {
		return nil
	}
	if err := ws.Rescan(); err != nil {
		return nil
	}
	ws.SetOverlay(rel, []byte(text))
	defer ws.ClearOverlay(rel)
	snap := ws.Snapshot()
	var out []mutate.Problem
	for _, d := range snap.Diagnostics {
		if !d.Primary.Valid() || snap.Sources.Path(d.Primary.File) != rel {
			continue
		}
		line := strings.TrimSpace(diag.Format(snap.Sources, d))
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			line = line[:i]
		}
		out = append(out, mutate.Problem{Node: rel, Message: line})
	}
	return out
}
