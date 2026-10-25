package docstool

import (
	"flag"
	"fmt"
	"io"

	"aego/engine/module"
	"aego/project"
)

func Tool(p *project.Project, args []string, stdout, stderr io.Writer, mods ...module.Module) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: docs export [--out file] | docs check [--strict] | docs report [--json]")
		return 2
	}
	fs := flag.NewFlagSet("docs "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "", "write the export to this file instead of stdout")
	strict := fs.Bool("strict", false, "undocumented project bindings are errors")
	asJSON := fs.Bool("json", false, "report as JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	build := Registry
	if args[0] == "export" {
		build = ExportRegistry
	}
	reg, err := build(p, mods)
	if err != nil {
		fmt.Fprintln(stderr, "aego:", err)
		return 1
	}
	switch args[0] {
	case "export":
		if err := WriteExport(reg, Title(p), *out, stdout); err != nil {
			fmt.Fprintln(stderr, "aego:", err)
			return 1
		}
		return 0
	case "check":
		return Check(reg, *strict, stdout, append(DiagnosticProblems(), GoProblems(reg, "", *strict)...)...)
	case "report":
		return Report(reg, "", stdout, *asJSON)
	}
	fmt.Fprintf(stderr, "unknown docs command %q\n", args[0])
	return 2
}
