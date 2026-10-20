package schema

import (
	"strings"
	"testing"
	"unsafe"
)

type cloneList struct {
	Tags []string `aego:"tags"`
	Size float32  `aego:"size"`
}

type cloneFlat struct {
	Size float32 `aego:"size"`
}

type cloneSkipped struct {
	Size  float32 `aego:"size"`
	Cache []int   `aego:"-"`
}

func TestCloneIntoCopiesLists(t *testing.T) {
	td, err := Register[cloneList](NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	if td.Flat {
		t.Fatal("a type with a list is marked flat")
	}
	src := cloneList{Tags: []string{"a", "b"}, Size: 2}
	var dst cloneList
	td.CloneInto(unsafe.Pointer(&dst), unsafe.Pointer(&src))
	dst.Tags[0] = "x"
	if src.Tags[0] != "a" || dst.Size != 2 || len(dst.Tags) != 2 {
		t.Fatalf("clone shares the list or lost a field: src %v dst %v", src, dst)
	}
}

func TestFlatTypeIsFlat(t *testing.T) {
	td, err := Register[cloneFlat](NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	if !td.Flat {
		t.Fatal("a scalar type is not flat")
	}
}

func TestSkippedSliceNeedsClone(t *testing.T) {
	_, err := Register[cloneSkipped](NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "Cache") {
		t.Fatalf("got %v, want an error that names Cache", err)
	}
	called := false
	td, err := Register[cloneSkipped](NewRegistry(), Clone(func(dst, src *cloneSkipped) {
		called = true
		dst.Cache = append([]int(nil), src.Cache...)
	}))
	if err != nil {
		t.Fatal(err)
	}
	src := cloneSkipped{Cache: []int{1, 2}}
	var dst cloneSkipped
	td.CloneInto(unsafe.Pointer(&dst), unsafe.Pointer(&src))
	dst.Cache[0] = 9
	if !called || src.Cache[0] != 1 {
		t.Fatal("the registered Clone did not run or the cache is shared")
	}
}
