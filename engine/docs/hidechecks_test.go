package docs

import "testing"

// Readers see the steps and the scripts; the scenes docs check plays stay out of the page.
func TestHideChecks(t *testing.T) {
	md := "Add a floor.\n\n```aescene check\nscene version=1 {}\n```\n\nThen a script:\n\n```aes check\npackage game\n```\n"
	got := HideChecks(md)
	want := "Add a floor.\n\nThen a script:\n\n```aes check\npackage game\n```"
	if got != want {
		t.Fatalf("HideChecks =\n%q\nwant\n%q", got, want)
	}
	if s := snippets(md); len(s) != 2 || !s[0].Check {
		t.Fatalf("the check scene must stay in the snippets: %+v", s)
	}
}
