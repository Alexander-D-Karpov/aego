package vcs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	proj := filepath.Join(dir, "game")
	os.MkdirAll(filepath.Join(proj, "scenes"), 0o755)
	os.WriteFile(filepath.Join(proj, "scenes", "main.aescene"), []byte("v1\n"), 0o644)
	run("add", ".")
	run("commit", "-q", "-m", "first")
	os.WriteFile(filepath.Join(proj, "scenes", "main.aescene"), []byte("v2\n"), 0o644)
	os.WriteFile(filepath.Join(proj, "new.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "outside.txt"), []byte("x"), 0o644)
	return proj
}

func TestGitShowStatusLog(t *testing.T) {
	proj := gitRepo(t)
	p, ok := Detect(proj)
	if !ok {
		t.Fatal("the project is inside a git repository")
	}
	data, err := p.Show("HEAD", "scenes/main.aescene")
	if err != nil || string(data) != "v1\n" {
		t.Fatalf("show = %q, %v", data, err)
	}
	st, err := p.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st["scenes/main.aescene"] != Modified || st["new.txt"] != Untracked {
		t.Fatalf("status = %v", st)
	}
	if _, leaked := st["outside.txt"]; leaked || len(st) != 2 {
		t.Fatalf("status must stay inside the project: %v", st)
	}
	log, err := p.Log("scenes/main.aescene", 10)
	if err != nil || len(log) != 1 || log[0].Subject != "first" {
		t.Fatalf("log = %+v, %v", log, err)
	}
}

func TestDetectOutsideRepo(t *testing.T) {
	if _, ok := Detect(t.TempDir()); ok {
		t.Fatal("a plain folder has no provider")
	}
}
