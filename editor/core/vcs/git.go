package vcs

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"aego/core/errs"
	"aego/core/log"
)

const gitTimeout = 10 * time.Second

type git struct {
	bin    string
	top    string
	root   string
	prefix string
}

func openGit(projectRoot string) (*git, bool) {
	bin, err := exec.LookPath("git")
	if err != nil {
		return nil, false
	}
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, false
	}
	g := &git{bin: bin, root: abs}
	out, err := g.run(abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, false
	}
	g.top = filepath.Clean(strings.TrimSpace(string(out)))
	top, err := filepath.EvalSymlinks(g.top)
	if err != nil {
		top = g.top
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		real = abs
	}
	rel, err := filepath.Rel(top, real)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, false
	}
	if rel != "." {
		g.prefix = filepath.ToSlash(rel) + "/"
	}
	return g, true
}

func (g *git) Name() string {
	return "git"
}

func (g *git) Root() string {
	return g.top
}

func (g *git) run(dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, g.bin, append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, errs.New(errs.EditorCommand, "git "+args[0]+" failed: "+msg, log.F("dir", dir))
	}
	return stdout.Bytes(), nil
}

func (g *git) repoPath(rel string) string {
	return g.prefix + strings.TrimPrefix(filepath.ToSlash(rel), "/")
}

func (g *git) projectPath(repo string) (string, bool) {
	if g.prefix == "" {
		return repo, true
	}
	return strings.CutPrefix(repo, g.prefix)
}

func (g *git) Show(rev, rel string) ([]byte, error) {
	if rev == "" {
		rev = "HEAD"
	}
	return g.run(g.top, "show", rev+":"+g.repoPath(rel))
}

func statusOf(x, y byte) FileStatus {
	switch {
	case x == '?' && y == '?':
		return Untracked
	case x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D'):
		return Conflicted
	case x == 'R' || y == 'R':
		return Renamed
	case x == 'A':
		return Added
	case x == 'D' || y == 'D':
		return Deleted
	}
	return Modified
}

func (g *git) Status() (map[string]FileStatus, error) {
	args := []string{"status", "--porcelain=v1", "-z", "--untracked-files=all"}
	if g.prefix != "" {
		args = append(args, "--", strings.TrimSuffix(g.prefix, "/"))
	}
	out, err := g.run(g.top, args...)
	if err != nil {
		return nil, err
	}
	return parseStatus(out, g.projectPath), nil
}

func parseStatus(out []byte, rel func(string) (string, bool)) map[string]FileStatus {
	res := map[string]FileStatus{}
	parts := bytes.Split(out, []byte{0})
	for i := 0; i < len(parts); i++ {
		e := parts[i]
		if len(e) < 4 {
			continue
		}
		st := statusOf(e[0], e[1])
		name := string(e[3:])
		if st == Renamed {
			i++
		}
		if p, ok := rel(name); ok {
			res[p] = st
		}
	}
	return res
}

func (g *git) Log(rel string, n int) ([]Commit, error) {
	if n <= 0 {
		n = 50
	}
	out, err := g.run(g.top, "log", "-n", strconv.Itoa(n), "--format=%H%x1f%h%x1f%an%x1f%at%x1f%s%x1e", "--", g.repoPath(rel))
	if err != nil {
		return nil, err
	}
	return parseLog(out), nil
}

func parseLog(out []byte) []Commit {
	var res []Commit
	for _, rec := range strings.Split(string(out), "\x1e") {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		f := strings.Split(rec, "\x1f")
		if len(f) < 5 {
			continue
		}
		sec, _ := strconv.ParseInt(f[3], 10, 64)
		res = append(res, Commit{Hash: f[0], Short: f[1], Author: f[2], When: time.Unix(sec, 0), Subject: f[4]})
	}
	return res
}
