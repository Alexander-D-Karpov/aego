package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type releaseGitBlob struct {
	path string
	oid  string
	mode string
}

// anyOutput discards Git's path list: an ignored path may itself be sensitive,
// and an unexpectedly large directory must not create an unbounded buffer.
type anyOutput struct{ present bool }

func (o *anyOutput) Write(p []byte) (int, error) {
	o.present = o.present || len(p) != 0
	return len(p), nil
}

// verifyReleaseSource checks the actual input bytes against the pinned Git tree,
// not just Git status: ignored files can participate in go:embed, and index flags
// can hide modified tracked files. Snapshot builds deliberately do not use this
// check. These observations are best-effort and are not a filesystem CAS; the
// trusted CI must still use an isolated checkout and a controlled environment.
func verifyReleaseSource(root, commit string) error {
	if !commitPattern.MatchString(commit) {
		return fmt.Errorf("release source needs a full Git commit")
	}
	top, err := command(root, nil, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return err
	}
	// Remove only Git's line ending: spaces may be part of a checkout path.
	topPath := strings.TrimSuffix(strings.TrimSuffix(string(top), "\n"), "\r")
	topInfo, err := os.Stat(topPath)
	if err != nil || !os.SameFile(rootInfo, topInfo) {
		return fmt.Errorf("release source must be the Git checkout root")
	}
	head, err := command(root, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(head)) != commit {
		return fmt.Errorf("release source HEAD changed")
	}
	status, err := command(root, nil, "git", "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(status)) != 0 {
		return fmt.Errorf("release source requires an exact clean Git tree; use a snapshot for working changes")
	}
	// A clean status alone does not expose assume-unchanged or skip-worktree.
	flags, err := command(root, nil, "git", "ls-files", "-v", "-z")
	if err != nil {
		return err
	}
	for _, item := range bytes.Split(flags, []byte{0}) {
		if len(item) != 0 && (len(item) < 3 || item[0] != 'H' || item[1] != ' ') {
			return fmt.Errorf("release source index contains hidden or unsupported entries")
		}
	}
	inputs := append(append([]string(nil), studioSourceDirs...), "go.mod", "go.sum")
	// No --exclude-standard: Git must report ignored files too. A go.work in
	// the repository or its parent must not substitute an external module.
	unknown := append(append([]string(nil), inputs...), "go.work", "go.work.sum")
	args := append([]string{"ls-files", "--others", "-z", "--"}, unknown...)
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	var output anyOutput
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cannot inspect release source inputs: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if output.present {
		return fmt.Errorf("untracked or ignored release source input is present")
	}
	workspace, err := command(root, nil, "go", "env", "GOWORK")
	if err != nil {
		return err
	}
	if value := strings.TrimSpace(string(workspace)); value != "" && value != "off" {
		return fmt.Errorf("release source cannot use an ambient Go workspace")
	}
	args = append([]string{"git", "ls-tree", "-r", "-z", "--full-tree", commit, "--"}, inputs...)
	tree, err := command(root, nil, args...)
	if err != nil {
		return err
	}
	var blobs []releaseGitBlob
	seen := make(map[string]bool)
	for _, raw := range bytes.Split(tree, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		header, path, ok := strings.Cut(string(raw), "\t")
		fields := strings.Fields(header)
		if !ok || len(fields) != 3 || (fields[0] != "100644" && fields[0] != "100755") ||
			fields[1] != "blob" || !commitPattern.MatchString(fields[2]) ||
			!fs.ValidPath(path) || strings.Contains(path, "\\") || !filepath.IsLocal(filepath.FromSlash(path)) || seen[path] {
			return fmt.Errorf("unsupported release source entry in Git tree")
		}
		seen[path] = true
		blobs = append(blobs, releaseGitBlob{path: path, oid: fields[2], mode: fields[0]})
	}
	if !seen["go.mod"] || !seen["go.sum"] {
		return fmt.Errorf("release source Git tree is missing module inputs")
	}
	return verifyReleaseBlobs(root, blobs)
}

// Compare bytes directly to Git objects through one batch process. The SHA-256
// sourceHash is still checked separately; this comparison binds those source
// inputs to the actual pinned commit instead of relying on Git stat shortcuts.
func verifyReleaseBlobs(root string, blobs []releaseGitBlob) (result error) {
	cmd := exec.Command("git", "cat-file", "--batch")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		_ = in.Close()
		return err
	}
	if err := cmd.Start(); err != nil {
		_ = in.Close()
		_ = out.Close()
		return err
	}
	defer func() {
		_ = in.Close()
		if result != nil {
			_ = out.Close()
			_ = cmd.Process.Kill()
		}
		if err := cmd.Wait(); result == nil && err != nil {
			result = fmt.Errorf("cannot verify release source Git objects: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
		_ = out.Close()
	}()
	reader := bufio.NewReader(out)
	buffer := make([]byte, 64<<10)
	for _, blob := range blobs {
		file := filepath.Join(root, filepath.FromSlash(blob.path))
		info, err := os.Lstat(file)
		if err != nil {
			return fmt.Errorf("cannot identify release source %s: %w", blob.path, err)
		}
		if !info.Mode().IsRegular() || runtime.GOOS != "windows" && (info.Mode().Perm()&0o111 != 0) != (blob.mode == "100755") {
			return fmt.Errorf("release source mode differs from Git commit: %s", blob.path)
		}
		data, err := readBounded(file, maxFileBytes)
		if err != nil {
			return fmt.Errorf("cannot read release source %s: %w", blob.path, err)
		}
		if _, err := fmt.Fprintln(in, blob.oid); err != nil {
			return err
		}
		header, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		fields := strings.Fields(header)
		if len(fields) != 3 || fields[0] != blob.oid || fields[1] != "blob" {
			return fmt.Errorf("release source Git object is unavailable: %s", blob.path)
		}
		length, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || length != int64(len(data)) {
			return fmt.Errorf("release source differs from Git commit: %s", blob.path)
		}
		for offset := 0; offset < len(data); {
			n := min(len(buffer), len(data)-offset)
			if _, err := io.ReadFull(reader, buffer[:n]); err != nil {
				return err
			}
			if !bytes.Equal(buffer[:n], data[offset:offset+n]) {
				return fmt.Errorf("release source differs from Git commit: %s", blob.path)
			}
			offset += n
		}
		if newline, err := reader.ReadByte(); err != nil || newline != '\n' {
			return fmt.Errorf("invalid release source Git object framing")
		}
	}
	return nil
}
