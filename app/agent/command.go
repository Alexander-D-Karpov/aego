package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"

	"aego/scene/mutate"
)

type Arg struct {
	Name     string
	Type     string
	Doc      string
	Required bool
	Pos      bool
	Items    map[string]any
}

type Command struct {
	Name  string
	Doc   string
	Args  []Arg
	Write bool
	// Destroys marks commands that remove or move things; the editor never applies them without asking (D342).
	Destroys bool
	// Look is how the editor's Agent panel shows a call; every command has one.
	Look Look
	Run  func(s *Session, a Args) (*Output, error)
}

type Args map[string]any

type Output struct {
	OK       bool             `json:"ok"`
	DryRun   bool             `json:"dryRun,omitempty"`
	Changes  []mutate.Change  `json:"changes,omitempty"`
	Problems []mutate.Problem `json:"problems,omitempty"`
	Created  []string         `json:"created,omitempty"`
	Files    []string         `json:"files,omitempty"`
	Notes    []string         `json:"notes,omitempty"`
	Value    any              `json:"value,omitempty"`
	Text     string           `json:"-"`
	// Images go to the model as image content, such as the screenshots of a run.
	Images  []Image      `json:"-"`
	Drawing *DrawingEdit `json:"-"`
	Art     ArtProposal  `json:"-"`
}

// Image is a picture an answer carries.
type Image struct {
	Mime string
	Data []byte
}

var dryRun = Arg{Name: "dry-run", Type: "boolean", Doc: "report the changes without writing anything"}

var all = catalog()

func catalog() []*Command {
	var out []*Command
	out = append(out, sceneCommands()...)
	out = append(out, prefabCommands()...)
	out = append(out, dataCommands()...)
	out = append(out, animCommands()...)
	out = append(out, ctrlCommands()...)
	out = append(out, refsCommands()...)
	out = append(out, assetCommands()...)
	out = append(out, variantCommand())
	out = append(out, materialCommands()...)
	out = append(out, postEffectCommands()...)
	out = append(out, textureCommands()...)
	out = append(out, artCommands()...)
	out = append(out, schemaCommands()...)
	out = append(out, projectCommands()...)
	out = append(out, scriptCommands()...)
	ls := looks()
	for _, c := range out {
		c.Look = ls[c.Name]
		if c.Name == "asset.import-variant" {
			c.Look = ls["asset.pixels"]
		}
		if c.Look.Title == nil && (strings.HasPrefix(c.Name, "postfx.") || strings.HasPrefix(c.Name, "material.") || strings.HasPrefix(c.Name, "shader.") || strings.HasPrefix(c.Name, "texture.") || strings.HasPrefix(c.Name, "art.")) {
			name := strings.ReplaceAll(c.Name, ".", " ")
			c.Look = Look{Icon: "comp.sprite", Title: func(a Args) string { return name + " " + file(a) }, Summary: changes}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func Commands() []*Command {
	return all
}

func Find(name string) (*Command, bool) {
	for _, c := range all {
		if c.Name == name {
			return c, true
		}
	}
	return nil, false
}

func Groups() []string {
	var out []string
	for _, c := range all {
		g, _, _ := strings.Cut(c.Name, ".")
		if !slices.Contains(out, g) {
			out = append(out, g)
		}
	}
	return out
}

func (c *Command) Tool() string {
	return strings.NewReplacer(".", "_", "-", "_").Replace(c.Name)
}

func (c *Command) AllArgs() []Arg {
	if c.Write {
		return append(slices.Clone(c.Args), dryRun)
	}
	return c.Args
}

func (c *Command) Usage() string {
	parts := strings.Split(c.Name, ".")
	for _, x := range c.Args {
		switch {
		case x.Pos && x.Required:
			parts = append(parts, "<"+x.Name+">")
		case x.Pos:
			parts = append(parts, "[<"+x.Name+">]")
		case x.Type == "boolean":
			parts = append(parts, "[--"+x.Name+"]")
		default:
			parts = append(parts, "[--"+x.Name+" v]")
		}
	}
	if c.Write {
		parts = append(parts, "[--dry-run]")
	}
	if c.Name == "art.capture" {
		parts = append(parts, "[--out path]")
	}
	return strings.Join(parts, " ")
}

func (c *Command) Schema() map[string]any {
	props := map[string]any{}
	var req []string
	for _, x := range c.AllArgs() {
		prop := map[string]any{"type": x.Type, "description": x.Doc}
		if x.Items != nil {
			prop["items"] = x.Items
		}
		props[x.Name] = prop
		if x.Required {
			req = append(req, x.Name)
		}
	}
	sch := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(req) > 0 {
		sch["required"] = req
	}
	return sch
}

func (c *Command) Check(a Args) error {
	known := map[string]bool{}
	for _, x := range c.AllArgs() {
		known[x.Name] = true
		if x.Required && !a.Has(x.Name) {
			return fmt.Errorf("%s needs %s", c.Name, x.Name)
		}
	}
	for k := range a {
		if !known[k] {
			return fmt.Errorf("%s has no argument %s", c.Name, k)
		}
	}
	return nil
}

func Run(s *Session, name string, a Args) (*Output, error) {
	c, ok := Find(name)
	if !ok {
		return nil, fmt.Errorf("unknown command %s", name)
	}
	if err := c.Check(a); err != nil {
		return nil, err
	}
	out, err := c.Run(s, a)
	if out != nil {
		out.OK = err == nil && !out.Errors()
	}
	return out, err
}

func (a Args) Str(name string) string {
	switch v := a[name].(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case json.Number:
		return v.String()
	}
	b, err := json.Marshal(a[name])
	if err != nil {
		return fmt.Sprint(a[name])
	}
	return string(b)
}

func (a Args) Has(name string) bool {
	_, ok := a[name]
	return ok && a.Str(name) != ""
}

func (a Args) Bool(name string) bool {
	b, _ := strconv.ParseBool(a.Str(name))
	return b
}

func (a Args) Int(name string, def int) (int, error) {
	if !a.Has(name) {
		return def, nil
	}
	n, err := strconv.Atoi(a.Str(name))
	if err != nil {
		return 0, fmt.Errorf("%s takes a whole number", name)
	}
	return n, nil
}

func (o *Output) Errors() bool {
	for _, p := range o.Problems {
		if p.Error {
			return true
		}
	}
	return false
}

func (o *Output) Print(w io.Writer) {
	switch {
	case o.Text != "":
		io.WriteString(w, o.Text)
		if !strings.HasSuffix(o.Text, "\n") {
			fmt.Fprintln(w)
		}
	case o.Value != nil:
		b, _ := json.MarshalIndent(o.Value, "", "  ")
		w.Write(append(b, '\n'))
	}
	for _, c := range o.Changes {
		line := c.Op + " " + c.Node
		if c.Path != "" {
			line += " " + c.Path
		}
		if c.Before != "" || c.After != "" {
			line += ": " + c.Before + " → " + c.After
		}
		fmt.Fprintln(w, line)
	}
	for _, p := range o.Problems {
		level := "warning"
		if p.Error {
			level = "error"
		}
		if p.Node != "" {
			fmt.Fprintf(w, "%s: %s: %s\n", level, p.Node, p.Message)
			continue
		}
		fmt.Fprintf(w, "%s: %s\n", level, p.Message)
	}
	for _, n := range o.Notes {
		fmt.Fprintln(w, n)
	}
	switch {
	case o.DryRun:
		fmt.Fprintln(w, "dry run: nothing written")
	case len(o.Files) > 0:
		fmt.Fprintln(w, "wrote "+strings.Join(o.Files, ", "))
	}
}

func Markdown(w io.Writer) {
	MarkdownIn(w, "Command", "Meaning", func(c *Command) string { return c.Doc })
}

// MarkdownIn is the command table of the CLI reference with its headings and descriptions in another language.
func MarkdownIn(w io.Writer, command, meaning string, doc func(c *Command) string) {
	fmt.Fprintln(w, "| "+command+" | "+meaning+" |")
	fmt.Fprintln(w, "|---|---|")
	for _, c := range all {
		fmt.Fprintf(w, "| `aego %s` | %s |\n", strings.ReplaceAll(c.Usage(), "|", `\|`), doc(c))
	}
}

// DocKey is the catalog key of a command's description, for the translated CLI reference.
func DocKey(c *Command) string {
	return "cli.agent." + c.Name
}
