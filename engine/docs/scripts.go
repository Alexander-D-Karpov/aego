package docs

import "strings"

const (
	ProjectID     = "project"
	ScriptsPrefix = "scripts/"
)

type ScriptSymbol struct {
	Package   string
	Container string
	Name      string
	Kind      string
	Signature string
	Doc       string
	Path      string
	Line      int
}

func scriptKind(k string) Kind {
	switch strings.ToLower(k) {
	case "script", "type", "struct":
		return Type
	case "signal":
		return Signal
	case "method":
		return Method
	case "field", "var", "property":
		return Field
	case "const", "constant":
		return Const
	case "enum":
		return Enum
	}
	return Function
}

func (r *Registry) scripts(project string, syms []ScriptSymbol) {
	if len(syms) == 0 {
		return
	}
	if project == "" {
		project = "Project"
	}
	r.add(Entry{
		ID: ProjectID, Kind: Page, Owner: project, Title: project,
		Body: "Script packages of " + project + ", built from the doc comments above scripts, signals and exported declarations.",
	})
	seen := map[string]bool{}
	for _, s := range syms {
		pkg := ScriptsPrefix + s.Package
		if !seen[pkg] {
			seen[pkg] = true
			r.add(Entry{ID: pkg, Parent: ProjectID, Kind: Module, Owner: project, Title: s.Package})
		}
		parent := pkg
		if s.Container != "" {
			parent = pkg + "." + s.Container
		}
		r.add(Entry{
			ID: parent + "." + s.Name, Parent: parent, Kind: scriptKind(s.Kind), Owner: project, Title: s.Name,
			Signature: s.Signature, Body: s.Doc, Location: Location{Path: s.Path, Line: s.Line},
		})
	}
}
