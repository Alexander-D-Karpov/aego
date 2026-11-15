//go:build aego_editor

package docsview

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"aego/engine/docs/democheck"
)

// traceTryRunner streams progress before opening each real component environment.
type traceTryRunner struct {
	runner democheck.Runner
	output io.Writer
}

var _ democheck.Runner = traceTryRunner{}

func (r traceTryRunner) Open(ctx context.Context, contract democheck.Contract, env string) (democheck.Instance, error) {
	if r.output != nil {
		fmt.Fprintf(r.output, "component-try open component=%q environment=%q\n", contract.ComponentID, env)
	}
	instance, err := r.runner.Open(ctx, contract, env)
	if r.output != nil {
		state := "ready"
		if err != nil {
			state = "error"
		} else if instance == nil {
			state = "no-instance"
		}
		fmt.Fprintf(r.output, "component-try open-result component=%q environment=%q state=%s\n", contract.ComponentID, env, state)
	}
	return instance, err
}

type traceRunnerFunc func(context.Context, democheck.Contract, string) (democheck.Instance, error)

func (f traceRunnerFunc) Open(ctx context.Context, contract democheck.Contract, env string) (democheck.Instance, error) {
	return f(ctx, contract, env)
}

type traceTestInstance struct{}

func (*traceTestInstance) Observe(context.Context) (democheck.Observation, error) {
	return democheck.Observation{}, nil
}
func (*traceTestInstance) Action(context.Context, democheck.Action) error { return nil }
func (*traceTestInstance) Reset(context.Context, string) error            { return nil }
func (*traceTestInstance) Close() error                                   { return nil }

func TestTraceTryRunnerPreservesResultsAndStreamsBeforeOpen(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "failure"
		}
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			want := &traceTestInstance{}
			failure := errors.New("underlying open failed")
			ctx := context.Background()
			contract := democheck.Contract{ComponentID: "engine.Example"}
			runner := traceTryRunner{output: &output, runner: traceRunnerFunc(func(gotCtx context.Context, got democheck.Contract, env string) (democheck.Instance, error) {
				if gotCtx != ctx || got.ComponentID != contract.ComponentID || env != "firefox" {
					t.Fatal("runner changed the open request")
				}
				if got := output.String(); got != "component-try open component=\"engine.Example\" environment=\"firefox\"\n" {
					t.Fatalf("start was not streamed before opening: %q", got)
				}
				if fail {
					return nil, failure
				}
				return want, nil
			})}
			got, err := runner.Open(ctx, contract, "firefox")
			if fail {
				if got != nil || err != failure || !strings.Contains(output.String(), "state=error\n") {
					t.Fatalf("failure changed: instance=%v error=%v trace=%q", got, err, output.String())
				}
			} else if got != want || err != nil || !strings.Contains(output.String(), "state=ready\n") {
				t.Fatalf("success changed: instance=%v error=%v trace=%q", got, err, output.String())
			}
		})
	}
}

type traceErrorWriter struct{}

func (traceErrorWriter) Write([]byte) (int, error) { return 0, errors.New("trace is unavailable") }

func TestTraceTryRunnerDoesNotChangeOpenOnTraceFailure(t *testing.T) {
	want := &traceTestInstance{}
	runner := traceRunnerFunc(func(context.Context, democheck.Contract, string) (democheck.Instance, error) {
		return want, nil
	})
	for _, output := range []io.Writer{nil, traceErrorWriter{}} {
		got, err := (traceTryRunner{runner: runner, output: output}).Open(context.Background(), democheck.Contract{}, "native")
		if got != want || err != nil {
			t.Fatalf("trace failure changed the runner result: instance=%v error=%v", got, err)
		}
	}
}

func TestTraceTryRunnerEscapesNames(t *testing.T) {
	var output bytes.Buffer
	runner := traceTryRunner{output: &output, runner: traceRunnerFunc(func(context.Context, democheck.Contract, string) (democheck.Instance, error) {
		return &traceTestInstance{}, nil
	})}
	if _, err := runner.Open(context.Background(), democheck.Contract{ComponentID: "engine.Example\nnext"}, "firefox"); err != nil {
		t.Fatal(err)
	}
	if strings.Count(output.String(), "\n") != 2 || !strings.Contains(output.String(), `engine.Example\nnext`) {
		t.Fatalf("component name broke the progress log: %q", output.String())
	}
}

func TestTraceTryRunnerDoesNotClaimMissingInstanceIsReady(t *testing.T) {
	var output bytes.Buffer
	runner := traceTryRunner{output: &output, runner: traceRunnerFunc(func(context.Context, democheck.Contract, string) (democheck.Instance, error) {
		return nil, nil
	})}
	got, err := runner.Open(context.Background(), democheck.Contract{}, "native")
	if got != nil || err != nil || !strings.Contains(output.String(), "state=no-instance\n") {
		t.Fatalf("missing instance changed or misreported: instance=%v error=%v trace=%q", got, err, output.String())
	}
}
