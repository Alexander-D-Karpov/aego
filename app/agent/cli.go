package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"aego/project"
)

type cliFlags struct {
	json   bool
	quiet  bool
	strict bool
}

func Usage(w io.Writer) {
	fmt.Fprintln(w, "usage: aego <group> <verb> <args> [--json] [--dry-run] [--strict] [--quiet] [--project dir]")
	fmt.Fprintln(w)
	for _, c := range all {
		fmt.Fprintf(w, "  %-58s %s\n", c.Usage(), c.Doc)
	}
}

func Main(p *project.Project, args []string, stdout, stderr io.Writer, opt Options) int {
	if len(args) < 2 {
		Usage(stderr)
		return 2
	}
	if args[1] == "help" {
		Usage(stdout)
		return 0
	}
	c, ok := Find(args[0] + "." + args[1])
	if !ok {
		fmt.Fprintf(stderr, "aego: unknown command %s %s\n", args[0], args[1])
		Usage(stderr)
		return 2
	}
	a, f, err := parseArgs(c, args[2:])
	if err == nil {
		err = c.Check(a)
	}
	if err != nil {
		fmt.Fprintln(stderr, "aego:", err)
		fmt.Fprintln(stderr, "usage: aego "+c.Usage())
		return 2
	}
	if addr, token, ok := EditorProxy(p.Root); ok {
		out, err := proxyRun(addr, token, c, a)
		if !errors.As(err, new(errProxyDown)) {
			return report(stdout, stderr, f, out, err)
		}
	}
	s, err := Open(p, opt)
	if err != nil {
		return report(stdout, stderr, f, nil, err)
	}
	defer s.Close()
	out, err := Run(s, c.Name, a)
	return report(stdout, stderr, f, out, err)
}

func parseArgs(c *Command, toks []string) (Args, cliFlags, error) {
	var f cliFlags
	a := Args{}
	byName := map[string]Arg{}
	var pos []Arg
	for _, x := range c.AllArgs() {
		byName[x.Name] = x
		if x.Pos {
			pos = append(pos, x)
		}
	}
	var free []string
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t == "--" {
			free = append(free, toks[i+1:]...)
			break
		}
		if !strings.HasPrefix(t, "--") {
			free = append(free, t)
			continue
		}
		name, val, has := strings.Cut(t[2:], "=")
		switch name {
		case "json":
			f.json = true
			continue
		case "quiet":
			f.quiet = true
			continue
		case "strict":
			f.strict = true
			continue
		case "project":
			if !has {
				i++
			}
			continue
		}
		x, ok := byName[name]
		if !ok {
			return nil, f, fmt.Errorf("unknown flag --%s", name)
		}
		if !has {
			if x.Type == "boolean" {
				val = "true"
			} else {
				if i+1 >= len(toks) {
					return nil, f, fmt.Errorf("--%s needs a value", name)
				}
				i++
				val = toks[i]
			}
		}
		a[name] = val
	}
	if len(free) > len(pos) {
		return nil, f, fmt.Errorf("too many arguments")
	}
	for i, v := range free {
		a[pos[i].Name] = v
	}
	return a, f, nil
}

func report(stdout, stderr io.Writer, f cliFlags, out *Output, err error) int {
	switch {
	case f.json:
		var v any = out
		if err != nil {
			v = map[string]any{"ok": false, "error": err.Error()}
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(v)
	case err != nil:
		fmt.Fprintln(stderr, "aego:", err)
	case !f.quiet:
		out.Print(stdout)
	}
	switch {
	case err != nil:
		return 1
	case out.Errors(), f.strict && len(out.Problems) > 0:
		return 2
	}
	return 0
}
