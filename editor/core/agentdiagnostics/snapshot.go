// Package agentdiagnostics formats a bounded attachment from diagnostics, not instructions.
package agentdiagnostics

import (
	"encoding/json"
	"sort"
	"strings"
)

type Row struct {
	Code       string `json:"code"`
	File       string `json:"file,omitempty"`
	Line       int    `json:"line,omitempty"`
	Column     int    `json:"column,omitempty"`
	Message    string `json:"message"`
	Node       string `json:"node_id,omitempty"`
	BlocksPlay bool   `json:"blocks_play,omitempty"`
}

func cut(s string, n int) string {
	s = strings.ToValidUTF8(s, "�")
	if len(s) <= n {
		return s
	}
	for i := range s {
		if i >= n {
			return s[:i] + "…"
		}
	}
	return s
}
func Snapshot(rows []Row) string {
	rows = append([]Row(nil), rows...)
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.BlocksPlay != b.BlocksPlay {
			return a.BlocksPlay
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Node != b.Node {
			return a.Node < b.Node
		}
		return a.Message < b.Message
	})
	n := len(rows)
	if n > 8 {
		rows = rows[:8]
	}
	for i := range rows {
		r := &rows[i]
		r.File = cut(r.File, 250)
		r.Message = cut(r.Message, 380)
		r.Code = cut(r.Code, 64)
		r.Node = cut(r.Node, 64)
	}
	b, _ := json.Marshal(struct {
		Total int   `json:"total"`
		Items []Row `json:"diagnostics"`
	}{n, rows})
	return string(b)
}
