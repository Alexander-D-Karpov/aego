package run

import (
	"os"

	"aego/core/trace"
)

func WriteTrace(path string, t *trace.Tracer) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := trace.WriteChrome(f, t, t.Snapshot(t.Frames())); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
