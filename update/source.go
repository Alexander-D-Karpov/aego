package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	// CheckTimeout bounds the whole check, including the manifest, signature and retries.
	CheckTimeout   = 20 * time.Second
	requestTimeout = 4 * time.Second
	checkAttempts  = 3
	// ReleaseFile and SigFile are what a channel publishes.
	ReleaseFile = "release.json"
	SigFile     = "release.json.sig"
)

// Source is where releases come from: <BaseURL>/<channel>/release.json and its signature. Aego uses its Gitea releases, a game its own server.
type Source struct {
	BaseURL string
	Channel Channel
	// Key is the base64 public key; empty uses PublicKey.
	Key string
	// Cache keeps the last release.json and its ETag, so an unchanged release costs a 304.
	Cache  string
	Client *http.Client
}

func (s *Source) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: requestTimeout}
}

func (s *Source) url(file string) string {
	ch := s.Channel
	if ch == "" {
		ch = Stable
	}
	return strings.TrimSuffix(s.BaseURL, "/") + "/" + string(ch) + "/" + file
}

// Check fetches the channel's release.json and verifies its signature; nothing is trusted without it.
func (s *Source) Check(ctx context.Context) (*Release, error) {
	key := s.Key
	if key == "" {
		key = PublicKey
	}
	pub, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, CheckTimeout)
	defer cancel()
	data, err := s.fetchRelease(ctx)
	if err != nil {
		return nil, err
	}
	sig, _, err := s.get(ctx, s.url(SigFile), "")
	if err != nil {
		return nil, err
	}
	return Verify(data, sig, pub)
}

// fetchRelease is release.json, from the cache when the server says it has not changed.
func (s *Source) fetchRelease(ctx context.Context) ([]byte, error) {
	body, etag := s.cached()
	data, tag, err := s.get(ctx, s.url(ReleaseFile), etag)
	if errors.Is(err, errNotModified) && body != nil {
		return body, nil
	}
	if err != nil {
		return nil, err
	}
	s.store(data, tag)
	return data, nil
}

var errNotModified = errors.New("not modified")

func (s *Source) get(ctx context.Context, url, etag string) ([]byte, string, error) {
	var err error
	for attempt := 0; attempt < checkAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		if attempt > 0 {
			timer := time.NewTimer(time.Duration(attempt) * 200 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, "", ctx.Err()
			case <-timer.C:
			}
		}
		var data []byte
		var tag string
		var retry bool
		data, tag, retry, err = s.getOnce(ctx, url, etag)
		if err == nil || !retry || ctx.Err() != nil {
			if ctx.Err() != nil {
				return nil, "", ctx.Err()
			}
			return data, tag, err
		}
	}
	return nil, "", err
}

func transientCheckError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	var dns *net.DNSError
	if errors.As(err, &dns) && dns.IsTemporary {
		return true
	}
	var network net.Error
	return errors.As(err, &network) && network.Timeout()
}

func (s *Source) getOnce(ctx context.Context, url, etag string) ([]byte, string, bool, error) {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", false, err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, "", transientCheckError(err), err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotModified:
		return nil, "", false, errNotModified
	case http.StatusNotFound:
		return nil, "", false, fmt.Errorf("no %s release at %s", s.Channel, url)
	default:
		retry := resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 && resp.StatusCode <= 599
		return nil, "", retry, fmt.Errorf("%s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20+1))
	if err != nil {
		return nil, "", transientCheckError(err), err
	}
	if len(data) > 1<<20 {
		return nil, "", false, errors.New("release response exceeds 1 MiB")
	}
	return data, resp.Header.Get("ETag"), false, nil
}

func (s *Source) cacheFiles() (string, string) {
	if s.Cache == "" {
		return "", ""
	}
	name := string(s.Channel)
	if name == "" {
		name = string(Stable)
	}
	return filepath.Join(s.Cache, name+".json"), filepath.Join(s.Cache, name+".etag")
}

func (s *Source) cached() ([]byte, string) {
	body, tag := s.cacheFiles()
	if body == "" {
		return nil, ""
	}
	data, err := os.ReadFile(body)
	if err != nil {
		return nil, ""
	}
	etag, _ := os.ReadFile(tag)
	return data, strings.TrimSpace(string(etag))
}

func (s *Source) store(data []byte, etag string) {
	body, tag := s.cacheFiles()
	if body == "" || etag == "" {
		return
	}
	if os.MkdirAll(s.Cache, 0o755) == nil {
		_ = os.WriteFile(body, data, 0o644)
		_ = os.WriteFile(tag, []byte(etag), 0o644)
	}
}

// Progress reports a download: bytes done of total, total 0 when unknown.
type Progress func(done, total int64)

// Download fetches an asset into dir, resuming a partial file, and checks its sha256; a mismatch deletes the file.
func Download(ctx context.Context, client *http.Client, a *Asset, dir string, progress Progress) (string, error) {
	if a.SHA256 == "" {
		return "", ErrChecksum
	}
	if client == nil {
		client = &http.Client{}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := filepath.Base(a.URL)
	if i := strings.IndexAny(name, "?#"); i >= 0 {
		name = name[:i]
	}
	final := filepath.Join(dir, name)
	if ok, _ := matches(final, a.SHA256); ok {
		return final, nil
	}
	part := final + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	have, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return "", err
	}
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusPartialContent:
	case http.StatusOK:
		if have > 0 {
			if err := f.Truncate(0); err != nil {
				return "", err
			}
			have = 0
		}
	default:
		return "", fmt.Errorf("%s: %s", a.URL, resp.Status)
	}
	total := a.Size
	buf := make([]byte, 64<<10)
	done := have
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return "", err
			}
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", rerr
		}
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if ok, err := matches(part, a.SHA256); err != nil || !ok {
		os.Remove(part)
		if err != nil {
			return "", err
		}
		return "", ErrChecksum
	}
	if err := os.Rename(part, final); err != nil {
		return "", err
	}
	return final, nil
}

func matches(path, sum string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return strings.EqualFold(hex.EncodeToString(h.Sum(nil)), strings.TrimSpace(sum)), nil
}

// Sum is the sha256 of a file, as release.json records it.
func Sum(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}
