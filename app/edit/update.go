package edit

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"aego/editor/core/document"
	"aego/format/aetext"
)

const (
	ReleasesAPI  = "https://git.akarpov.ru/api/v1/repos/sanspie/aego"
	DownloadPage = "https://aego.akarpov.ru/install.html"
	updateFile   = "update"
	updateEvery  = 24 * time.Hour
	fetchTimeout = 8 * time.Second
)

type updateState struct {
	Enabled bool
	Checked time.Time
	Latest  string
}

type updater struct {
	file    string
	current string
	mu      sync.Mutex
	st      updateState
}

func updatePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "aego", updateFile)
}

func loadUpdate(file string) updateState {
	st := updateState{Enabled: true}
	data, err := os.ReadFile(file)
	if err != nil {
		return st
	}
	doc, err := aetext.Parse(data, updateFile)
	if err != nil {
		return st
	}
	if v, ok := doc.Attr("enabled"); ok {
		st.Enabled = v.Bool
	}
	if v, ok := doc.Attr("checked"); ok {
		st.Checked = time.Unix(v.AsInt(), 0)
	}
	if v, ok := doc.Attr("latest"); ok {
		st.Latest = v.Str
	}
	return st
}

func saveUpdate(file string, st updateState) error {
	n := &aetext.Node{Head: "update"}
	n.Attrs = append(n.Attrs,
		aetext.Field{Name: "enabled", Value: aetext.Bool(st.Enabled)},
		aetext.Field{Name: "checked", Value: aetext.Int(st.Checked.Unix())},
		aetext.Field{Name: "latest", Value: aetext.String(st.Latest)},
	)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return document.WriteAtomic(file, aetext.Write(n))
}

func newUpdater(file, current string) *updater {
	return &updater{file: file, current: current, st: loadUpdate(file)}
}

func (u *updater) start() {
	u.mu.Lock()
	due := u.file != "" && u.st.Enabled && time.Since(u.st.Checked) >= updateEvery
	u.mu.Unlock()
	if !due {
		return
	}
	go func() {
		api := os.Getenv("AEGO_API")
		if api == "" {
			api = ReleasesAPI
		}
		tag, err := fetchLatest(api)
		u.mu.Lock()
		defer u.mu.Unlock()
		u.st.Checked = time.Now()
		if err == nil {
			u.st.Latest = tag
		}
		saveUpdate(u.file, u.st)
	}()
}

func (u *updater) Available() (string, string, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if !u.st.Enabled || !newer(u.st.Latest, u.current) {
		return "", "", false
	}
	return strings.TrimPrefix(u.st.Latest, "v"), DownloadPage, true
}

func (u *updater) Enabled() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.st.Enabled
}

func (u *updater) SetEnabled(on bool) {
	u.mu.Lock()
	u.st.Enabled = on
	st := u.st
	u.mu.Unlock()
	if u.file != "" {
		saveUpdate(u.file, st)
	}
	if on {
		u.start()
	}
}

func fetchLatest(api string) (string, error) {
	c := http.Client{Timeout: fetchTimeout}
	resp, err := c.Get(api + "/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("releases API answered " + resp.Status)
	}
	var rel struct {
		Tag        string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	return rel.Tag, nil
}

func parseVersion(s string) ([3]int, string, bool) {
	var out [3]int
	core, pre, _ := strings.Cut(strings.TrimPrefix(strings.TrimSpace(s), "v"), "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return out, "", false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, "", false
		}
		out[i] = n
	}
	return out, pre, true
}

func newer(latest, current string) bool {
	a, ap, ok1 := parseVersion(latest)
	b, bp, ok2 := parseVersion(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	switch {
	case ap == bp:
		return false
	case ap == "":
		return true
	case bp == "":
		return false
	}
	return ap > bp
}
