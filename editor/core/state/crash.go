package state

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"aego/format/aetext"
)

type CrashReport struct {
	Dir     string
	Kind    string
	Where   string
	Time    string
	Message string
	Path    string
	Line    int
}

func CrashReports(root string) []CrashReport {
	base := filepath.Join(root, ".aego", "crash")
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var out []CrashReport
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(base, e.Name())
		r := CrashReport{Dir: dir, Time: e.Name()}
		if data, err := os.ReadFile(filepath.Join(dir, "info.aetext")); err == nil {
			if doc, err := aetext.Parse(data, "info.aetext"); err == nil {
				for _, f := range doc.Fields {
					switch f.Name {
					case "kind":
						r.Kind = f.Value.Str
					case "where":
						r.Where = f.Value.Str
					case "message":
						r.Message = f.Value.Str
					}
				}
			}
		}
		if data, err := os.ReadFile(filepath.Join(dir, "script.stack")); err == nil {
			first, _, _ := strings.Cut(string(data), "\n")
			loc, _, _ := strings.Cut(first, " ")
			if i := strings.LastIndexByte(loc, ':'); i > 0 {
				r.Path = loc[:i]
				r.Line, _ = strconv.Atoi(loc[i+1:])
			}
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time > out[j].Time })
	return out
}
