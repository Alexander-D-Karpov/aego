package agentui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	Dir        = ".aego/user/agent"
	ext        = ".jsonl"
	idLayout   = "20060102-150405"
	maxLineLen = 16 << 20
)

// Entry is one line of a session log.
type Entry struct {
	Images    []string  `json:"images,omitempty"`
	Time      time.Time `json:"time"`
	Kind      string    `json:"kind"`
	Text      string    `json:"text,omitempty"`
	Tool      string    `json:"tool,omitempty"`
	CallID    string    `json:"call,omitempty"`
	Args      string    `json:"args,omitempty"`
	Failed    bool      `json:"failed,omitempty"`
	Card      int       `json:"card,omitempty"`
	State     string    `json:"state,omitempty"`
	Provider  string    `json:"provider,omitempty"`
	Delta     bool      `json:"delta,omitempty"`
	MessageID string    `json:"message_id,omitempty"`
}

// Transcript appends the entries of one session to .aego/user/agent/<id>.jsonl.
type Transcript struct {
	ID string
	f  *os.File
}

func dir(root string) string {
	return filepath.Join(root, filepath.FromSlash(Dir))
}

func NewTranscript(root string, now time.Time) (*Transcript, error) {
	if err := os.MkdirAll(dir(root), 0o755); err != nil {
		return nil, err
	}
	base := now.Format(idLayout)
	for n := 0; ; n++ {
		id := base
		if n > 0 {
			id = fmt.Sprintf("%s-%06d", base, n)
		}
		f, err := os.OpenFile(filepath.Join(dir(root), id+ext), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return &Transcript{ID: id, f: f}, nil
	}
}

func (t *Transcript) Append(e Entry) error {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = t.f.Write(append(b, '\n'))
	return err
}

func (t *Transcript) Close() error {
	return t.f.Close()
}

// List is the ids of the saved sessions, newest first.
func List(root string) ([]string, error) {
	list, err := os.ReadDir(dir(root))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range list {
		if name := e.Name(); !e.IsDir() && strings.HasSuffix(name, ext) {
			out = append(out, strings.TrimSuffix(name, ext))
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out, nil
}

// Load reads a session; lines it cannot read are skipped.
func Load(root, id string) ([]Entry, error) {
	f, err := os.Open(filepath.Join(dir(root), id+ext))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), maxLineLen)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}
