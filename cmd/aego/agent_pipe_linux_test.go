package main

import (
	"encoding/json"
	"os"
	"syscall"
	"testing"
)

func TestAgentSceneTreeJSONSmallPipe(t *testing.T) {
	dir := copyProject(t, "platformer")
	var capacity uintptr
	code, out := captureStdout(t, func() int {
		var errno syscall.Errno
		capacity, _, errno = syscall.Syscall(syscall.SYS_FCNTL, os.Stdout.Fd(), syscall.F_SETPIPE_SZ, 4096)
		if errno != 0 {
			t.Fatalf("set pipe capacity: %v", errno)
		}
		return dispatch([]string{"scene", "tree", "scenes/main.aescene", "--json", "--project", dir})
	})
	if code != 0 || len(out) <= int(capacity) {
		t.Fatalf("exit %d, output %d bytes, pipe capacity %d", code, len(out), capacity)
	}
	var result struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(out, &result); err != nil || !result.OK {
		t.Fatalf("output %v:\n%s", err, out)
	}
}
