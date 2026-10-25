package docstool

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"aego/engine/docs"
)

// ReportRow is one page of aego docs report (D347).
type ReportRow struct {
	ID       string         `json:"id"`
	Group    string         `json:"group"`
	Intro    int            `json:"introWords"`
	Examples int            `json:"examples"`
	Patterns int            `json:"patterns"`
	Checks   map[string]int `json:"checks"`
	Edited   string         `json:"edited"`
	Red      bool           `json:"red"`
}

var docCode = regexp.MustCompile(`^DOC\d{3}\b`)

// Report lists every page with its opening paragraph, examples, style findings and last edit; a red row has errors in strict mode.
func Report(reg *docs.Registry, engine string, w io.Writer, asJSON bool) int {
	problems := append(reg.Check(true), GoProblems(reg, engine, true)...)
	byID := map[string][]docs.Problem{}
	for _, p := range problems {
		byID[p.ID] = append(byID[p.ID], p)
	}
	dates := map[string]string{}
	var rows []ReportRow
	red := 0
	for _, e := range reg.All() {
		if e.EditorOnly || e.Kind != docs.Page && e.Kind != docs.Module && e.Kind != docs.Code {
			continue
		}
		row := ReportRow{ID: e.ID, Group: e.Group, Intro: docs.IntroWords(e.Body), Examples: len(e.Snippets), Checks: map[string]int{}}
		for _, p := range byID[e.ID] {
			code := docCode.FindString(p.Message)
			switch {
			case code == "DOC010":
				row.Patterns++
			case code != "":
				row.Checks[code]++
			default:
				row.Checks["other"]++
			}
			row.Red = row.Red || p.Error
		}
		if src := e.Location.Source; src != "" && engine != "" {
			if _, ok := dates[src]; !ok {
				dates[src] = lastEdit(engine, src)
			}
			row.Edited = dates[src]
		}
		if row.Red {
			red++
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Red != rows[j].Red {
			return rows[i].Red
		}
		return rows[i].ID < rows[j].ID
	})
	if asJSON {
		data, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Fprintln(w, string(data))
	} else {
		tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, "\tpage\tgroup\tintro\texamples\tpatterns\tchecks\tedited")
		for _, r := range rows {
			mark := ""
			if r.Red {
				mark = "RED"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\t%s\t%s\n", mark, r.ID, r.Group, r.Intro, r.Examples, r.Patterns, checks(r.Checks), r.Edited)
		}
		tw.Flush()
		fmt.Fprintf(w, "docs report: %d pages, %d red\n", len(rows), red)
	}
	return 0
}

func checks(m map[string]int) string {
	var parts []string
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%s×%d", k, v))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

// lastEdit is the date of the last commit that touched a file, or "" without git.
func lastEdit(root, path string) string {
	cmd := exec.Command("git", "log", "-1", "--format=%cs", "--", path)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
