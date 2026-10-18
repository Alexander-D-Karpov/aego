package coroutine

import (
	"testing"

	"aego/script/vm"
)

func TestHandleWaitWakesOnComplete(t *testing.T) {
	s := New(vm.New(vm.Options{}))
	s.Yielded(vm.CoHandle{Index: 0, Gen: 1}, vm.WaitHandle, vm.Value{A: 7, B: 9})
	if st := s.Stats(); st.Awaiting != 1 || st.Sleeping != 0 {
		t.Fatalf("parked %+v", st)
	}
	s.Complete(9, 8)
	s.Complete(3, 7)
	if s.Stats().Awaiting != 1 {
		t.Fatal("another handle woke the coroutine")
	}
	s.Complete(9, 7)
	if st := s.Stats(); st.Awaiting != 0 || st.Ready != 1 {
		t.Fatalf("after complete %+v", st)
	}
	s.Frame(0)
	if s.Stats().Ready != 0 {
		t.Fatal("woken waits must drain on the next frame")
	}
}

func TestNilHandleWaitsOneFrame(t *testing.T) {
	s := New(vm.New(vm.Options{}))
	s.Yielded(vm.CoHandle{Index: 0, Gen: 1}, vm.WaitHandle, vm.Value{})
	if st := s.Stats(); st.Awaiting != 0 || st.Sleeping != 1 {
		t.Fatalf("nil handle %+v", st)
	}
}
