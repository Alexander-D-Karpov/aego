package edit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"aego/app/agent"
	"aego/app/run"
	"aego/project"
)

// agentRunTimeout bounds one headless run the agent asks for.
const agentRunTimeout = 2 * time.Minute

// agentRun runs the game headless in a child process, so the editor's language, audio and asset index stay as they are.
func agentRun(p *project.Project, r agent.RunRequest) (agent.RunReport, error) {
	exe, err := os.Executable()
	if err != nil {
		return agent.RunReport{}, err
	}
	args := []string{"--tool", "agent-run", "--frames", strconv.Itoa(r.Frames), "--seed", strconv.FormatUint(r.Seed, 10)}
	if r.Scene != "" {
		args = append(args, "--scene", r.Scene)
	}
	if len(r.Watch) > 0 {
		args = append(args, "--watch", strings.Join(r.Watch, ","))
	}
	ctx, cancel := context.WithTimeout(context.Background(), agentRunTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = p.Root
	cmd.Env = append(os.Environ(), "SDL_AUDIO_DRIVER=dummy")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	rerr := cmd.Run()
	var res run.AgentRunResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return agent.RunReport{}, errors.New("the run took longer than " + agentRunTimeout.String())
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" && rerr != nil {
			msg = rerr.Error()
		}
		return agent.RunReport{}, errors.New("the game did not run: " + msg)
	}
	if res.Err != "" {
		return res.Report, errors.New(res.Err)
	}
	return res.Report, nil
}
