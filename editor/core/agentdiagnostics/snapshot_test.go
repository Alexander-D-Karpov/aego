package agentdiagnostics

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSnapshotIsBoundedAndStable(t *testing.T) {
	var rows []Row
	for i := 12; i > 0; i-- {
		rows = append(rows, Row{Code: "AES", File: "игра.aes", Line: i, Message: strings.Repeat("ошибка ", 110), BlocksPlay: i == 11})
	}
	data := Snapshot(rows)
	var v struct {
		Total       int
		Diagnostics []Row
	}
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		t.Fatal(err)
	}
	if v.Total != 12 || len(v.Diagnostics) != 8 || v.Diagnostics[0].Line != 11 || v.Diagnostics[1].Line != 1 || rows[0].Line != 12 || len(v.Diagnostics[0].Message) > 390 || !utf8.ValidString(data) {
		t.Fatalf("bad snapshot: %s", data)
	}
}
