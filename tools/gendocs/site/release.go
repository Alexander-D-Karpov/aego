package site

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Release struct {
	Tag    string  `json:"tag_name"`
	URL    string  `json:"html_url"`
	Assets []Asset `json:"assets"`
}

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// download is one file offered for an OS, in the order the page prefers them.
type download struct {
	os, label string
	match     func(name string) bool
}

var downloads = []download{
	{"windows", "Windows installer", func(n string) bool { return strings.HasSuffix(n, "-windows-amd64-setup.exe") }},
	{"windows", "Windows installer (.zip)", func(n string) bool { return strings.HasSuffix(n, "-windows-amd64-setup.zip") }},
	{"windows", "Windows portable (.zip)", func(n string) bool { return strings.HasSuffix(n, "-windows-amd64.zip") }},
	{"macos", "macOS (.dmg)", func(n string) bool { return strings.HasSuffix(n, ".dmg") }},
	{"linux", "Linux AppImage", func(n string) bool { return strings.HasSuffix(n, ".AppImage") }},
	{"linux", "Linux (.tar.gz)", func(n string) bool { return strings.HasSuffix(n, "-linux-amd64.tar.gz") }},
}

// FetchRelease reads the latest release of a Gitea repository, such as https://git.example/owner/repo.
func FetchRelease(repo string) (*Release, error) {
	u, err := url.Parse(strings.TrimRight(repo, "/"))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("bad repository URL %q", repo)
	}
	api := u.Scheme + "://" + u.Host + "/api/v1/repos" + u.Path + "/releases/latest"
	c := http.Client{Timeout: 10 * time.Second}
	resp, err := c.Get(api)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", api, resp.Status)
	}
	var r Release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

func sizeMB(n int64) string {
	return strconv.FormatFloat(float64(n)/(1<<20), 'f', 0, 64) + " MB"
}

// downloadBlock is the per-OS download button; site.js points it at the visitor's OS.
func (b *builder) downloadBlock(root string) string {
	r := b.opt.Release
	if r == nil {
		return ""
	}
	var items []string
	for _, d := range downloads {
		for _, a := range r.Assets {
			if d.match(a.Name) {
				items = append(items, `<li data-os="`+d.os+`"><a href="`+esc(a.URL)+`">`+esc(d.label)+`</a><span>`+sizeMB(a.Size)+"</span></li>")
				break
			}
		}
	}
	if len(items) == 0 {
		return ""
	}
	fallback := r.URL
	if u, ok := b.urls[installPage]; ok {
		fallback = root + u
	}
	return `<div class="download" data-fallback="` + esc(fallback) + `"><div class="dl-head"><a class="button primary dl-main" href="` + esc(r.URL) + `">` + b.icon(root, "file.save", "") + `<span>Download</span></a>` +
		`<span class="dl-meta">` + esc(r.Tag) + ` · <a href="` + esc(r.URL) + `">all files</a></span></div><ul class="dl-list">` + strings.Join(items, "") + "</ul></div>\n"
}
