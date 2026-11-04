package edit

import (
	"io/fs"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

type webPreview struct {
	mu       sync.Mutex
	server   *http.Server
	root     *os.Root
	listener net.Listener
	dir, url string
	cleanup  func()
}

func (p *webPreview) stop() {
	if p.server != nil {
		_ = p.listener.Close()
		_ = p.server.Close()
		p.server = nil
	}
	if p.root != nil {
		_ = p.root.Close()
		p.root = nil
	}
	if p.cleanup != nil {
		p.cleanup()
		p.cleanup = nil
	}
	p.dir, p.url = "", ""
}
func (p *webPreview) close() { p.mu.Lock(); defer p.mu.Unlock(); p.stop() }

func (p *webPreview) serve(dir string, cleanup func(), valid func() bool) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if valid != nil && !valid() {
		return "", fs.ErrClosed
	}
	if dir == p.dir {
		return p.url, nil
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		root.Close()
		return "", err
	}
	files := http.FileServerFS(root.FS())
	host := ln.Addr().String()
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host {
			http.Error(w, "invalid preview host", http.StatusForbidden)
			return
		}
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		w.Header().Set("Cache-Control", "no-store")
		files.ServeHTTP(w, r)
	})}
	p.stop()
	p.server, p.root, p.listener, p.cleanup, p.dir, p.url = server, root, ln, cleanup, dir, "http://"+host+"/"
	go server.Serve(ln)
	return p.url, nil
}
