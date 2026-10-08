package supervise

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"aego/editor/session"
	"aego/project"
)

const (
	BuildDir   = ".aego/build/editor"
	NativeDir  = "native"
	pollEvery  = 400 * time.Millisecond
	quietTime  = 300 * time.Millisecond
	crashGrace = 10 * time.Second
)

type Options struct {
	Debug      bool
	Hidden     bool
	CPUProfile string
	Stdout     io.Writer
	Stderr     io.Writer
}

type supervisor struct {
	p       *project.Project
	opt     Options
	dir     string
	gen     int
	current string
	prev    string
	next    string
	started time.Time
}

func Run(p *project.Project, opt Options) error {
	if opt.Stdout == nil {
		opt.Stdout = os.Stdout
	}
	if opt.Stderr == nil {
		opt.Stderr = os.Stderr
	}
	s := &supervisor{p: p, opt: opt, dir: filepath.Join(p.Root, filepath.FromSlash(BuildDir))}
	session.Clear(p.Root)
	fmt.Fprintln(opt.Stderr, "aego: building the editor with native modules")
	if err := s.generate(); err != nil {
		return err
	}
	bin, out, err := s.build()
	if err != nil {
		return fmt.Errorf("native editor build failed:\n%s", out)
	}
	s.current = bin
	return s.loop()
}

func (s *supervisor) say(format string, args ...any) {
	fmt.Fprintf(s.opt.Stderr, "aego: "+format+"\n", args...)
}

func (s *supervisor) loop() error {
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	exited := make(chan int, 1)
	if err := s.start(exited); err != nil {
		return err
	}
	defer s.cleanup()
	fp := s.fingerprint()
	var changedAt time.Time
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()
	for {
		select {
		case <-interrupts:
		case code := <-exited:
			done, err := s.exited(code, exited)
			if done {
				return err
			}
		case <-tick.C:
			if cur := s.fingerprint(); cur != fp {
				fp = cur
				changedAt = time.Now()
				continue
			}
			if !changedAt.IsZero() && time.Since(changedAt) >= quietTime {
				changedAt = time.Time{}
				s.rebuild()
			}
		}
	}
}

func (s *supervisor) exited(code int, exited chan int) (bool, error) {
	switch {
	case code == session.ExitRestart:
		if s.next != "" {
			s.prev, s.current, s.next = s.current, s.next, ""
		}
		if err := s.start(exited); err != nil {
			return true, err
		}
		return false, nil
	case code == 0:
		return true, nil
	case s.prev != "" && time.Since(s.started) < crashGrace:
		s.say("the rebuilt editor exited with code %d, starting the previous build", code)
		s.current, s.prev = s.prev, ""
		if err := s.start(exited); err != nil {
			return true, err
		}
		return false, nil
	}
	return true, fmt.Errorf("editor exited with code %d", code)
}

func (s *supervisor) start(exited chan<- int) error {
	var args []string
	if s.opt.Debug {
		args = append(args, "-debug")
	}
	if s.opt.Hidden {
		args = append(args, "-hidden")
	}
	if s.opt.CPUProfile != "" {
		args = append(args, "-cpuprofile", s.opt.CPUProfile)
	}
	args = append(args, s.p.Root)
	cmd := exec.Command(s.current, args...)
	cmd.Env = append(os.Environ(), session.EnvSupervised+"=1")
	cmd.Stdin = os.Stdin
	cmd.Stdout = s.opt.Stdout
	cmd.Stderr = s.opt.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot start the editor: %w", err)
	}
	s.started = time.Now()
	go func() {
		exited <- exitCode(cmd.Wait())
	}()
	return nil
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if c := ee.ExitCode(); c >= 0 {
			return c
		}
	}
	return 1
}

func (s *supervisor) rebuild() {
	s.say("native code changed, rebuilding the editor")
	session.WriteState(s.p.Root, session.StateBuilding, "")
	if err := s.generate(); err != nil {
		s.say("%v", err)
		session.WriteState(s.p.Root, session.StateError, err.Error())
		return
	}
	bin, out, err := s.build()
	if err != nil {
		fmt.Fprint(s.opt.Stderr, out)
		session.WriteState(s.p.Root, session.StateError, out)
		return
	}
	if s.next != "" {
		os.Remove(s.next)
	}
	s.next = bin
	session.WriteState(s.p.Root, session.StateOK, "")
	if err := session.RequestRestart(s.p.Root); err != nil {
		s.say("cannot request the restart: %v", err)
	}
}

func (s *supervisor) build() (string, string, error) {
	s.gen++
	name := "aego-editor-" + strconv.Itoa(s.gen)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(s.dir, name)
	cmd := exec.Command("go", "build", "-tags", "aego_editor", "-o", out, filepath.Join(s.dir, "main.go"))
	cmd.Dir = s.p.Root
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		os.Remove(out)
		return "", buf.String(), err
	}
	return out, buf.String(), nil
}

func (s *supervisor) generate() error {
	mod, err := modulePath(s.p.Root)
	if err != nil {
		return err
	}
	calls, err := entryCalls(filepath.Join(s.p.Root, NativeDir))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, "main.go"), mainSource(mod+"/"+NativeDir, calls), 0o644)
}

func (s *supervisor) fingerprint() [sha256.Size]byte {
	h := sha256.New()
	add := func(name string, info os.FileInfo) {
		fmt.Fprintf(h, "%s|%d|%d\n", name, info.Size(), info.ModTime().UnixNano())
	}
	filepath.WalkDir(filepath.Join(s.p.Root, NativeDir), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		if info, err := d.Info(); err == nil {
			add(p, info)
		}
		return nil
	})
	for _, name := range []string{"go.mod", "go.sum"} {
		if info, err := os.Stat(filepath.Join(s.p.Root, name)); err == nil {
			add(name, info)
		}
	}
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out
}

func (s *supervisor) cleanup() {
	for _, bin := range []string{s.prev, s.next} {
		if bin != "" {
			os.Remove(bin)
		}
	}
	session.ClearRestart(s.p.Root)
}
