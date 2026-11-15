package agent

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtCaptureCreatesNewFileOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.png")
	png := []byte("capture bytes")
	out := &Output{Images: []Image{{Mime: "image/png", Data: png}}}
	if err := writeArtCapture(path, out); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, png) {
		t.Fatalf("saved capture = %q, %v", got, err)
	}
	if err := writeArtCapture(path, &Output{Images: []Image{{Mime: "image/png", Data: []byte("replacement")}}}); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing capture error = %v, want ErrExist", err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, png) {
		t.Fatalf("existing capture changed = %q, %v", got, err)
	}
}

type captureReplacementWriter struct {
	file        *os.File
	path        string
	original    string
	replacement []byte
	failWrite   bool
	fault       error
}

func (w *captureReplacementWriter) Write(data []byte) (int, error) {
	if w.failWrite {
		n, err := w.file.Write(data[:len(data)/2])
		if err != nil {
			return n, err
		}
		return n, w.fault
	}
	return w.file.Write(data)
}

func (w *captureReplacementWriter) Close() error {
	if err := w.file.Close(); err != nil {
		return err
	}
	// Keep the opened file under another name, then simulate an external
	// process creating a replacement before error cleanup can run.
	if err := os.Rename(w.path, w.original); err != nil {
		return err
	}
	f, err := os.OpenFile(w.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(w.replacement); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if !w.failWrite {
		return w.fault
	}
	return nil
}

func TestArtCapturePreservesExternalReplacementOnFailure(t *testing.T) {
	for _, failWrite := range []bool{true, false} {
		name := "close"
		if failWrite {
			name = "write"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "capture.png")
			original := filepath.Join(dir, "original")
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			data := []byte("capture bytes")
			replacement := []byte("external replacement")
			fault := errors.New("injected capture error")
			writer := &captureReplacementWriter{file: file, path: path, original: original, replacement: replacement, failWrite: failWrite, fault: fault}
			if err := finishArtCapture(path, writer, data); !errors.Is(err, fault) || !strings.Contains(err.Error(), "incomplete output may remain") {
				t.Fatalf("capture error = %v, want injected error and incomplete-output warning", err)
			}
			wantOriginal := data
			if failWrite {
				wantOriginal = data[:len(data)/2]
			}
			if got, err := os.ReadFile(original); err != nil || !bytes.Equal(got, wantOriginal) {
				t.Fatalf("opened capture changed = %q, %v", got, err)
			}
			if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, replacement) {
				t.Fatalf("external replacement changed or removed = %q, %v", got, err)
			}
		})
	}
}

type shortCaptureWriter struct{ *os.File }

func (w shortCaptureWriter) Write(data []byte) (int, error) {
	return w.File.Write(data[:len(data)/2])
}

func TestArtCaptureKeepsPartialFileOnShortWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.png")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("capture bytes")
	if err := finishArtCapture(path, shortCaptureWriter{f}, data); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write error = %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, data[:len(data)/2]) {
		t.Fatalf("partial capture changed or removed = %q, %v", got, err)
	}
	if _, err := f.Write([]byte("after close")); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("capture file was not closed: %v", err)
	}
}
