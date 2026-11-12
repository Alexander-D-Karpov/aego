package pipeline

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFingerprintSourceBoundsAndDistinguishesMissingFromEmpty(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := newPipeline(root, nil, nil)
	var absent context.Context // Exercise the API's explicit nil-context rejection.
	if _, err := p.FingerprintSource(absent, "assets/empty.wav", 0); err == nil {
		t.Fatal("accepted a nil context")
	}
	var missingPipeline *Pipeline
	if _, err := missingPipeline.FingerprintSource(context.Background(), "assets/empty.wav", 0); err == nil {
		t.Fatal("accepted a nil pipeline")
	}
	missing, err := p.FingerprintSource(context.Background(), "assets/missing.wav", 0)
	if err != nil || missing.Exists || missing.Size != 0 || missing.SHA256 != [sha256.Size]byte{} {
		t.Fatalf("missing source: %+v %v", missing, err)
	}
	empty := filepath.Join(root, "assets", "empty.wav")
	if err := os.WriteFile(empty, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := p.FingerprintSource(context.Background(), "assets/empty.wav", 0)
	if err != nil || !got.Exists || got.Size != 0 || got.SHA256 != sha256.Sum256(nil) {
		t.Fatalf("empty source: %+v %v", got, err)
	}
	// A sparse synthetic source is larger than the existing 16 MiB source
	// transaction limit. The fingerprint reads it in bounded chunks only.
	const size = (16 << 20) + 37
	large := filepath.Join(root, "assets", "large.wav")
	f, err := os.OpenFile(large, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.FingerprintSource(context.Background(), "assets/large.wav", size-1); err == nil {
		t.Fatal("accepted a file exceeding the caller's limit")
	}
	h := sha256.New()
	zero := make([]byte, 64<<10)
	for remaining := size; remaining > 0; {
		n := min(remaining, len(zero))
		h.Write(zero[:n])
		remaining -= n
	}
	got, err = p.FingerprintSource(context.Background(), "assets/large.wav", size)
	var want [sha256.Size]byte
	copy(want[:], h.Sum(nil))
	if err != nil || !got.Exists || got.Size != size || got.SHA256 != want {
		t.Fatalf("large source: %+v %v", got, err)
	}
	if _, err := p.SourceBytes("assets/large.wav"); err == nil {
		t.Fatal("fingerprinting unexpectedly raised the transaction source limit")
	}
	if info, err := os.Stat(large); err != nil || info.Size() != size {
		t.Fatal("fingerprinting changed the source", err)
	}
	for _, limit := range []int64{-1, MaxSourceFingerprintBytes + 1} {
		if _, err := p.FingerprintSource(context.Background(), "assets/empty.wav", limit); err == nil {
			t.Fatalf("accepted invalid limit %d", limit)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.FingerprintSource(ctx, "assets/empty.wav", 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("ignored cancellation: %v", err)
	}
}

func TestFingerprintSourceRejectsUnsafePathsAndPendingRecovery(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "real.wav"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := newPipeline(root, nil, nil)
	for _, rel := range []string{"../outside", "assets/.hidden.wav", "assets/../real.wav", "assets/bad:name", "assets"} {
		if _, err := p.FingerprintSource(context.Background(), rel, 1024); err == nil {
			t.Errorf("accepted unsafe path %q", rel)
		}
	}
	t.Run("symlink", func(t *testing.T) {
		if err := os.Symlink("real.wav", filepath.Join(assets, "link.wav")); err != nil {
			t.Skipf("symlink creation unavailable: %v", err)
		}
		if _, err := p.FingerprintSource(context.Background(), "assets/link.wav", 1024); err == nil {
			t.Fatal("followed a symlink to another source")
		}
	})
	journal := filepath.Join(root, filepath.FromSlash(sourceJournalDir))
	if err := os.MkdirAll(journal, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := p.FingerprintSource(context.Background(), "assets/real.wav", 1024); err == nil {
		t.Fatal("fingerprinted while source recovery is pending")
	}
	if _, err := os.Stat(journal); err != nil {
		t.Fatal("fingerprinting changed pending recovery", err)
	}
}

func TestFingerprintSourceRejectsChangesAndCancellationDuringRead(t *testing.T) {
	for _, reason := range []string{"same-size replacement", "in-place edit", "directory replacement", "cancellation"} {
		t.Run(reason, func(t *testing.T) {
			root := t.TempDir()
			assets := filepath.Join(root, "assets")
			if err := os.MkdirAll(assets, 0o755); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(assets, "sample.wav")
			if err := os.WriteFile(file, []byte("original"), 0o644); err != nil {
				t.Fatal(err)
			}
			p := newPipeline(root, nil, nil)
			fsroot, err := os.OpenRoot(root)
			if err != nil {
				t.Fatal(err)
			}
			defer fsroot.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			checkpoint := func() {
				switch reason {
				case "same-size replacement":
					candidate := filepath.Join(assets, "candidate.wav")
					if err := os.WriteFile(candidate, []byte("external"), 0o644); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(candidate, file); err != nil {
						t.Skipf("replacement of an open file unavailable: %v", err)
					}
				case "in-place edit":
					if err := os.WriteFile(file, []byte("external"), 0o644); err != nil {
						t.Fatal(err)
					}
					future := time.Now().Add(10 * time.Second)
					if err := os.Chtimes(file, future, future); err != nil {
						t.Fatal(err)
					}
				case "directory replacement":
					if err := os.Rename(assets, filepath.Join(root, "old-assets")); err != nil {
						t.Skipf("replacement of an open directory unavailable: %v", err)
					}
					if err := os.Mkdir(assets, 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(file, []byte("external"), 0o644); err != nil {
						t.Fatal(err)
					}
				case "cancellation":
					cancel()
				}
			}
			got, err := p.fingerprintSource(ctx, fsroot, "assets/sample.wav", 1024, checkpoint)
			if err == nil || got.Exists || (reason == "cancellation" && !errors.Is(err, context.Canceled)) {
				t.Fatalf("accepted changed source: %+v %v", got, err)
			}
			want := "external"
			if reason == "cancellation" {
				want = "original"
			}
			if data, err := os.ReadFile(file); err != nil || string(data) != want {
				t.Fatal("fingerprinting changed the owner's file", err)
			}
		})
	}
}
