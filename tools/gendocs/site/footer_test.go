package site

import (
	"strings"
	"testing"
	"time"
)

func TestFooterAuthorNoticeOnEveryPage(t *testing.T) {
	_, _, pages := build(t, fixture(), Options{
		Version: "0.12.5-dev", Repo: "https://git.example/aego", Commit: "abc1234",
		Time: time.Date(2030, time.February, 1, 0, 0, 0, 0, time.UTC),
	})
	for name, page := range pages {
		t.Run(name, func(t *testing.T) {
			_, footer, ok := strings.Cut(page, `<footer class="foot">`)
			if !ok {
				t.Fatal("page has no documentation footer")
			}
			footer, _, ok = strings.Cut(footer, "</footer>")
			const notice = `Copyleft 2026 <a href="https://about.akarpov.ru/">akarpov</a>`
			if !ok || strings.Count(footer, notice) != 1 {
				t.Fatal("footer must contain exactly one fixed-year author notice and profile link")
			}
			for _, want := range []string{"Aego 0.12.5-dev", "abc1234", "2030-02-01", "llms.txt"} {
				if !strings.Contains(footer, want) {
					t.Errorf("author notice displaced existing footer metadata/link %q", want)
				}
			}
		})
	}
}

func TestFooterShortensGitHashButKeepsFullLink(t *testing.T) {
	const commit = "5fd892c50fb6950b609bbe86ef887cb80887d689"
	_, _, pages := build(t, fixture(), Options{Repo: "https://git.example/aego", Commit: commit})
	want := `<a href="https://git.example/aego/commit/` + commit + `">5fd892c</a>`
	for name, page := range pages {
		if !strings.Contains(page, want) {
			t.Errorf("%s: footer must link the full commit but display seven characters", name)
		}
	}
}

func TestShortCommitPreservesNonHashLabels(t *testing.T) {
	for input, want := range map[string]string{
		"": "", "abc1234": "abc1234", "abcdef012345": "abcdef0", "ABCDEF012345": "ABCDEF0",
		"example-dirty": "example-dirty", "1234567-unsafe": "1234567-unsafe",
	} {
		if got := shortCommit(input); got != want {
			t.Errorf("shortCommit(%q) = %q, want %q", input, got, want)
		}
	}
}
