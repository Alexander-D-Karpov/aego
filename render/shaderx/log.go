package shaderx

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var reLog = regexp.MustCompile(`(?m)^\s*(?:ERROR: |WARNING: )?[^:\n]*:(\d+)(?:\(\d+\))?: (.*)$`)

func MapLog(log string, locs []Loc, files []string, code string) []Diag {
	var out []Diag
	for _, m := range reLog.FindAllStringSubmatch(log, -1) {
		n, _ := strconv.Atoi(m[1])
		d := Diag{Code: code, Msg: strings.TrimSpace(m[2])}
		switch {
		case n >= 1 && n <= len(locs) && locs[n-1].File >= 0 && int(locs[n-1].File) < len(files):
			l := locs[n-1]
			d.File, d.Line = files[l.File], int(l.Line)
		case len(files) > 0:
			d.File = files[0]
		}
		if !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return out
}
