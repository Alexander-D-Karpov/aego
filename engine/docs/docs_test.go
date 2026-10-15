//go:build !aego_runtime

package docs

import "testing"

func TestPlain(t *testing.T) {
	md := "Text with [Clamp](ref:aego/math.Clamp) and []int.\n\n```aes\nx := 1\n```"
	want := "Text with Clamp and []int.\n\nx := 1"
	if got := Plain(md); got != want {
		t.Errorf("Plain = %q, want %q", got, want)
	}
}

func TestKeywordID(t *testing.T) {
	for in, want := range map[string]string{
		"yield wait": "keyword/yield-wait",
		"@export":    "keyword/export-attr",
		"Get<T>":     "keyword/get",
		"NodeRef":    "keyword/noderef",
	} {
		if got := KeywordID(in); got != want {
			t.Errorf("KeywordID(%q) = %q, want %q", in, got, want)
		}
	}
}
