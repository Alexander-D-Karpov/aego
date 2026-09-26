package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"

	"aego/app/run"
	"aego/asset/pipeline"
	"aego/engine"
	"aego/engine/bootstrap"
	"aego/project"
)

const (
	restartTimeout = 5 * time.Second
	rebuildQuiet   = 300 * time.Millisecond
)

type child struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	done   chan error
	binary string
}

func startChild(p *project.Project, binary string, args []string, restore bool) (*child, error) {
	full := append([]string{"--supervised"}, args...)
	if restore {
		if _, err := os.Stat(run.StatePath(p.Root)); err == nil {
			full = append(full, "--scene", run.StatePath(p.Root))
		}
	}
	cmd := exec.Command(binary, full...)
	cmd.Dir = p.Root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &child{cmd: cmd, stdin: stdin, done: make(chan error, 1), binary: binary}
	go func() { c.done <- cmd.Wait() }()
	return c, nil
}

func (c *child) restart() {
	io.WriteString(c.stdin, run.RestartCommand+"\n")
	select {
	case <-c.done:
	case <-time.After(restartTimeout):
		fmt.Fprintln(os.Stderr, "aego: game did not stop in time, killing it")
		c.cmd.Process.Kill()
		<-c.done
	}
	c.stdin.Close()
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return 1
}

func superviseNative(p *project.Project, args []string) int {
	build := func() (string, error) {
		return bootstrap.Build(p, bootstrap.HostTarget(), bootstrap.ModeRun, engine.Version, nil)
	}
	binary, err := build()
	if err != nil {
		return fail(err)
	}
	os.Remove(run.StatePath(p.Root))
	c, err := startChild(p, binary, args, false)
	if err != nil {
		return fail(err)
	}

	changes := make(chan struct{}, 1)
	stopWatch, err := pipeline.PollWatcher{}.Start(p.NativeDir(), nil, 150*time.Millisecond, func(cs []pipeline.Change) {
		for _, ch := range cs {
			if strings.HasSuffix(ch.Path, ".go") {
				select {
				case changes <- struct{}{}:
				default:
				}
				return
			}
		}
	})
	if err != nil {
		c.cmd.Process.Kill()
		return fail(err)
	}
	defer stopWatch()

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)

	var debounce <-chan time.Time
	for {
		select {
		case err := <-c.done:
			return exitCode(err)
		case <-interrupts:
			select {
			case <-c.done:
			case <-time.After(3 * time.Second):
				c.cmd.Process.Kill()
				<-c.done
			}
			return 130
		case <-changes:
			debounce = time.After(rebuildQuiet)
		case <-debounce:
			debounce = nil
			fmt.Fprintln(os.Stderr, "aego: native sources changed, rebuilding")
			next, err := build()
			if err != nil {
				fmt.Fprintln(os.Stderr, "aego: build failed, the running game is kept")
				fmt.Fprintln(os.Stderr, err)
				continue
			}
			if next == c.binary {
				continue
			}
			c.restart()
			nc, err := startChild(p, next, args, true)
			if err != nil {
				return fail(err)
			}
			c = nc
			fmt.Fprintln(os.Stderr, "aego: game restarted with the new build")
		}
	}
}
