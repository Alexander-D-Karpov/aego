package richtext

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWrappedTableLinkKeepsTextTargetAndCell(t *testing.T) {
	const id = "абв😀LongAssetабв😀LongAssetабв😀LongAsset"
	md := "| Asset | ID |\n|---|---|\n| [" + id + "](ref:asset/path) | 42 |"
	d := Layout(Parse(md), 120, mono7{}, nil)
	var value strings.Builder
	n := 0
	for _, row := range d.Lines {
		for _, run := range row.Runs {
			if run.Target != "ref:asset/path" {
				continue
			}
			if !utf8.ValidString(run.Text) || run.X+run.W > row.Cols[1]-CellPad+.01 {
				t.Fatalf("link fragment %q outside its cell", run.Text)
			}
			n++
			value.WriteString(run.Text)
		}
	}
	if n < 2 || value.String() != id {
		t.Fatalf("wrapped link %q in %d fragments, want %q", value.String(), n, id)
	}
}
