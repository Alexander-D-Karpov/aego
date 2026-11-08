//go:build linux

package edit

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/core/ids"
	"aego/editor/core/artedit"
	"aego/project"
)

const artCrashReady = "AEGO_ART_RECOVERY_WRITTEN"

// This uses a temporary project and a real headless editor subprocess. The
// parent kills it after a recovery snapshot has been written, then explicitly
// requests recovery; it does not simulate a power failure or multi-file save.
func TestArtRecoveryAfterForcedProcessTermination(t *testing.T) {
	if root := os.Getenv("AEGO_ART_CRASH_TEST_ROOT"); root != "" {
		artRecoveryCrashChild(t, root)
		return
	}
	t.Setenv("SDL_AUDIODRIVER", "dummy")
	t.Setenv("SDL_AUDIO_DRIVER", "dummy")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, conflict := range []bool{false, true} {
		name := "recover"
		if conflict {
			name = "external source conflict"
		}
		t.Run(name, func(t *testing.T) {
			p := newProject(t)
			id := ids.NewAssetID()
			drawing, err := art.New(16, 16, art.RGBA, art.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			original, err := drawing.Encode()
			drawing.Close()
			if err != nil {
				t.Fatal(err)
			}
			const rel = "assets/recovery.aeart"
			file := filepath.Join(p.Root, filepath.FromSlash(rel))
			if err := os.WriteFile(file, original, 0644); err != nil {
				t.Fatal(err)
			}
			imp := artsource.New()
			meta := asset.NewSourceMeta(imp.ID(), imp.Version(), imp.Defaults())
			meta.ID = id
			if err := os.WriteFile(file+".meta", asset.MarshalSourceMeta(meta), 0644); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestArtRecoveryAfterForcedProcessTermination$")
			cmd.Env = append(os.Environ(), "AEGO_ART_CRASH_TEST_ROOT="+p.Root, "AEGO_ART_CRASH_TEST_ID="+id.String())
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			ready := make(chan error, 1)
			go func() {
				scan := bufio.NewScanner(stdout)
				for scan.Scan() {
					if scan.Text() == artCrashReady {
						ready <- nil
						return
					}
				}
				ready <- fmt.Errorf("child exited before writing recovery: %v", scan.Err())
			}()
			select {
			case err := <-ready:
				if err != nil {
					_ = cmd.Wait()
					t.Fatalf("%v: %s", err, stderr.String())
				}
			case <-ctx.Done():
				_ = cmd.Wait()
				t.Fatalf("child timed out before recovery: %v; %s", ctx.Err(), stderr.String())
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = stdin.Close()
			if err := cmd.Wait(); err == nil || cmd.ProcessState.Success() {
				t.Fatal("editor subprocess was not forcibly terminated")
			}
			if got, err := os.ReadFile(file); err != nil || !bytes.Equal(got, original) {
				t.Fatal("the unsaved drawing changed the source before recovery")
			}
			if conflict {
				other, err := art.New(8, 8, art.RGBA, art.DefaultLimits())
				if err != nil {
					t.Fatal(err)
				}
				original, err = other.Encode()
				other.Close()
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, original, 0644); err != nil {
					t.Fatal(err)
				}
			}
			s, err := Open(p, headless())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if !s.Step() {
				t.Fatal("editor stopped on reopen")
			}
			doc, err := s.App.Context().OpenAsset(id)
			if err != nil {
				t.Fatal(err)
			}
			model, ok := doc.Binding.Model.(*artedit.Document)
			if !ok || !model.PendingRecovery || doc.Dirty() {
				t.Fatal("recovery was not offered explicitly or was applied automatically")
			}
			err = model.Recover() // This explicit call represents consent in the test.
			if conflict {
				if err == nil || !strings.Contains(err.Error(), "conflict") || !model.PendingRecovery {
					t.Fatalf("changed source did not block recovery: %v", err)
				}
			} else {
				if err != nil || !doc.Dirty() || model.PendingRecovery {
					t.Fatalf("explicit recovery did not restore a dirty document: %v", err)
				}
				img, _, err := model.Capture(model.Session.Frame, art.Albedo)
				if err != nil || img.NRGBAAt(4, 4).R != 255 || img.NRGBAAt(4, 4).A != 255 {
					t.Fatalf("recovered pixels differ: %v", err)
				}
			}
			if got, err := os.ReadFile(file); err != nil || !bytes.Equal(got, original) {
				t.Fatal("recovery or conflict handling overwrote the source")
			}
		})
	}
}

func artRecoveryCrashChild(t *testing.T, root string) {
	t.Helper()
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	id, err := ids.ParseAssetID(os.Getenv("AEGO_ART_CRASH_TEST_ID"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, headless())
	if err != nil {
		t.Fatal(err)
	}
	if !s.Step() {
		t.Fatal("editor stopped before editing")
	}
	doc, err := s.App.Context().OpenAsset(id)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok {
		t.Fatal("Art Studio did not open the source")
	}
	_, err = model.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 4, Y: 4}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}})
	if err != nil || !doc.Dirty() {
		t.Fatalf("could not create an unsaved stroke: %v", err)
	}
	if n, err := s.App.Context().Docs.WriteRecovery(); err != nil || n < 1 {
		t.Fatalf("recovery snapshot was not written: %d, %v", n, err)
	}
	fmt.Println(artCrashReady)
	// Keep the process alive for the parent to kill; no clean editor shutdown.
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
}
