package vm_test

import (
	"fmt"
	"testing"

	"aego/core/rewind"
	"aego/script/vm"
)

func TestHostStringCacheIsBoundedAndWeak(t *testing.T) {
	m := vm.New(vm.Options{})
	idle := m.HostString("Idle")
	for range 1000 {
		if got := m.HostString("Idle"); got != idle {
			t.Fatal("an unchanged host string allocated a new handle")
		}
	}
	if got := m.Stats().HeapObjects; got != 1 {
		t.Fatalf("heap objects = %d, want 1", got)
	}
	for i := range 32 {
		m.HostString(fmt.Sprintf("state-%d", i))
	}
	if next := m.HostString("Idle"); next == idle {
		t.Fatal("old entry was not evicted from the bounded cache")
	}
	m.Collect(nil)
	if got := m.Stats().HeapObjects; got != 0 {
		t.Fatalf("cache kept %d unreachable strings alive", got)
	}
	if _, ok := m.Str(idle); ok {
		t.Fatal("collected handle is still valid")
	}
	newIdle := m.HostString("Idle")
	if value, ok := m.Str(newIdle); !ok || value != "Idle" || newIdle == idle {
		t.Fatalf("stale cached handle returned: %q, %v", value, ok)
	}
	m.Collect(roots{newIdle})
	if value, ok := m.Str(newIdle); !ok || value != "Idle" {
		t.Fatalf("rooted host string lost: %q, %v", value, ok)
	}
}

func TestHostStringCacheRewindsWithTheHeap(t *testing.T) {
	m := vm.New(vm.Options{})
	m.HostString("Idle")
	var state vm.State
	m.Capture(&state)
	m.HostString("Run")
	h := rewind.NewHasher()
	m.Hash(h)
	want := h.Sum()
	for i := range 32 {
		m.HostString(fmt.Sprintf("later-%d", i))
	}
	m.Collect(nil)
	m.Restore(&state)
	m.HostString("Run")
	h.Reset()
	m.Hash(h)
	if got := h.Sum(); got != want {
		t.Fatalf("replayed VM state %x, want %x", got, want)
	}
}
