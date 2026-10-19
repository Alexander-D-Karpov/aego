package export

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"aego/engine/problems"
	"aego/runtime/bundle"
)

type Issue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	File     string `json:"file,omitempty"`
}

type Report struct {
	Target      string                `json:"target"`
	Profile     string                `json:"profile"`
	Out         string                `json:"out"`
	Binary      string                `json:"binary,omitempty"`
	BinarySize  int64                 `json:"binarySize"`
	Data        []bundle.Size         `json:"data"`
	DataSize    int64                 `json:"dataSize"`
	Artifacts   int                   `json:"artifacts"`
	Modules     []string              `json:"modules"`
	Millis      int64                 `json:"millis"`
	Hash        string                `json:"hash,omitempty"`
	Blocked     bool                  `json:"blocked"`
	Error       string                `json:"error,omitempty"`
	Issues      []Issue               `json:"issues"`
	Diagnostics []problems.Diagnostic `json:"-"`
}

func severity(s problems.Severity) string {
	switch s {
	case problems.Error:
		return "error"
	case problems.Warning:
		return "warning"
	}
	return "info"
}

func (r *Report) setDiagnostics(ds []problems.Diagnostic) {
	r.Diagnostics = ds
	r.Issues = make([]Issue, 0, len(ds))
	for _, d := range ds {
		r.Issues = append(r.Issues, Issue{Code: d.Code, Severity: severity(d.Severity), Message: d.Message, File: d.Subject.File})
	}
	r.Blocked = Blocking(ds) > 0
}

func Bytes(n int64) string {
	switch {
	case n >= 1<<30:
		return strconv.FormatFloat(float64(n)/(1<<30), 'f', 1, 64) + " GB"
	case n >= 1<<20:
		return strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64) + " MB"
	case n >= 1<<10:
		return strconv.FormatFloat(float64(n)/(1<<10), 'f', 1, 64) + " KB"
	}
	return strconv.FormatInt(n, 10) + " B"
}

func (r *Report) Lines() [][2]string {
	out := [][2]string{{"Target", r.Target}, {"Profile", r.Profile}, {"Output", r.Out}}
	if r.Binary != "" {
		out = append(out, [2]string{"Binary", r.Binary + " (" + Bytes(r.BinarySize) + ")"})
	}
	if r.Artifacts > 0 {
		out = append(out, [2]string{"Data", Bytes(r.DataSize) + " in " + strconv.Itoa(r.Artifacts) + " artifacts"})
		for _, s := range r.Data {
			out = append(out, [2]string{"  " + s.Kind, Bytes(s.Bytes) + ", " + strconv.Itoa(s.Count)})
		}
	}
	if len(r.Modules) > 0 {
		out = append(out, [2]string{"Modules", strings.Join(r.Modules, ", ")})
	}
	if r.Millis > 0 {
		out = append(out, [2]string{"Time", strconv.FormatFloat(float64(r.Millis)/1000, 'f', 1, 64) + " s"})
	}
	if len(r.Hash) >= 16 {
		out = append(out, [2]string{"Content hash", r.Hash[:16]})
	}
	return out
}

func (r *Report) Print(w io.Writer) {
	for _, is := range r.Issues {
		file := is.File
		if file == "" {
			file = "export"
		}
		fmt.Fprintf(w, "%s: %s %s: %s\n", file, is.Severity, is.Code, is.Message)
	}
	if r.Blocked {
		fmt.Fprintf(w, "export: blocked by %d problem(s)\n", Blocking(r.Diagnostics))
		return
	}
	if r.Error != "" || r.Binary == "" {
		return
	}
	width := 0
	lines := r.Lines()
	for _, l := range lines {
		width = max(width, len(l[0]))
	}
	for _, l := range lines {
		fmt.Fprintf(w, "%-*s  %s\n", width, l[0], l[1])
	}
}

func (r *Report) JSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
