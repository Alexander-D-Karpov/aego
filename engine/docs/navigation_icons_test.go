//go:build !aego_runtime

package docs

import "testing"

func TestNavigationIconsAreHostProvidedAndCopied(t *testing.T) {
	r := Assemble(Sources{})
	tasks := map[string]string{"guide": "file.art"}
	refs := map[string]string{"package": "cat.node"}
	r.SetNavigationIcons(tasks, refs)
	tasks["guide"] = "wrong"
	refs["package"] = "wrong"
	if got := r.NavigationTaskIcon("guide"); got != "file.art" {
		t.Fatalf("task icon = %q", got)
	}
	if got := r.NavigationReferenceIcon("package"); got != "cat.node" {
		t.Fatalf("reference icon = %q", got)
	}
	other := Assemble(Sources{})
	if other.NavigationTaskIcon("guide") != "" || other.NavigationReferenceIcon("package") != "" {
		t.Fatal("navigation policy leaked to another registry")
	}
	r.SetNavigationIcons(nil, nil)
	if r.NavigationTaskIcon("guide") != "" || r.NavigationReferenceIcon("package") != "" {
		t.Fatal("clearing a policy did not remove its icons")
	}
	var absent *Registry
	if absent.NavigationTaskIcon("guide") != "" || absent.NavigationReferenceIcon("package") != "" {
		t.Fatal("nil registry should not provide icon overrides")
	}
}
