// Art Studio development host and deterministic headless replay.
// This does not replace the engine CLI; it provides a separately lazy-loaded
// browser entrypoint for integration with the existing documentation build.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aego/art"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	addr := flag.String("addr", "127.0.0.1:8097", "loopback listen address")
	dir := flag.String("dir", ".cache/artstudio", "built browser directory")
	replay := flag.String("replay", "", "replay a JSON command fixture instead of serving")
	flag.Parse()
	if *replay != "" {
		return runReplay(*replay)
	}
	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("artstudio: use a loopback listen address")
	}
	root, err := os.Stat(*dir)
	if err != nil || !root.IsDir() {
		return fmt.Errorf("artstudio: run tools/artstudio/build.sh first")
	}
	handler := staticHandler(*dir)
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	fmt.Printf("Art Studio: http://%s\n", listener.Addr())
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	return server.Serve(listener)
}

// staticHandler serves only the explicit build outputs. It has no write/upload
// endpoint, and does not expose directory listings or unrelated working files.
func staticHandler(dir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			http.Error(w, "loopback host required", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		switch name {
		case "index.html", "studio.css", "studio.js", "recovery.js", "wasm_exec.js", "artstudio.wasm":
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' blob: data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		http.ServeFile(w, r, filepath.Join(dir, name))
	})
}

type replayFile struct {
	Width      int             `json:"width"`
	Height     int             `json:"height"`
	Mode       art.Mode        `json:"mode"`
	Operations []art.Operation `json:"operations"`
	Frame      art.ID          `json:"frame"`
}

func runReplay(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 4<<20+1))
	if err != nil {
		return err
	}
	if len(data) > 4<<20 {
		return fmt.Errorf("artstudio: replay exceeds limit")
	}
	var v replayFile
	if err = json.Unmarshal(data, &v); err != nil {
		return err
	}
	d, err := art.New(v.Width, v.Height, v.Mode, art.Limits{})
	if err != nil {
		return err
	}
	defer d.Close()
	h := art.NewJournal(art.Limits{})
	s, err := art.NewSession(d, h)
	if err != nil {
		return err
	}
	for i, op := range v.Operations {
		if _, err = s.Execute(context.Background(), op); err != nil {
			return fmt.Errorf("operation %d: %w", i, err)
		}
	}
	if v.Frame == "" {
		v.Frame = s.Frame
	}
	im, err := d.Render(v.Frame, art.RenderOptions{})
	if err != nil {
		return err
	}
	hash := sha256.Sum256(im.Pix)
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"pixelSHA256": hex.EncodeToString(hash[:]), "state": s.Info()})
}
