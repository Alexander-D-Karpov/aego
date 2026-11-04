package shader

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const SHD009 = "SHD009"

// Diag keeps a precise line when known; no invented column is exposed.
type Diag struct {
	Code    string
	File    string
	Line    int
	Msg     string
	Backend string
	Stage   string
	Variant uint32
	AssetID string
	RawLog  string
}

func (d Diag) Error() string {
	if d.Line > 0 {
		return fmt.Sprintf("%s:%d: %s: %s", d.File, d.Line, d.Code, d.Msg)
	}
	return fmt.Sprintf("%s: %s: %s", d.File, d.Code, d.Msg)
}

var reLog = regexp.MustCompile(`(?m)^\s*(?:ERROR:\s*|WARNING:\s*)?[^:\n]*:(\d+)(?:\(\d+\))?:\s*(.*)$`)
var reParenLog = regexp.MustCompile(`(?m)^\s*[^()\n]*\((\d+)\)\s*:\s*(.*)$`)

func MapLog(log string, locs []Loc, files []string, code string) []Diag {
	var out []Diag
	matches := reLog.FindAllStringSubmatch(log, -1)
	matches = append(matches, reParenLog.FindAllStringSubmatch(log, -1)...)
	for _, m := range matches {
		n, _ := strconv.Atoi(m[1])
		d := Diag{Code: code, Msg: strings.TrimSpace(m[2]), RawLog: log}
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
