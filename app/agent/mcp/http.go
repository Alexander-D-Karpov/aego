package mcp

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"time"
)

const maxBody = 8 << 20

// HTTP serves MCP as JSON-RPC over POST on loopback, for an agent the editor runs (D341).
type HTTP struct {
	URL   string
	Token string
	srv   *http.Server
	s     *Server
}

// ListenHTTP serves on 127.0.0.1 at a free port with a fresh bearer token.
func ListenHTTP(opt Options) (*HTTP, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		ln.Close()
		return nil, err
	}
	h := &HTTP{URL: "http://" + ln.Addr().String() + "/mcp", Token: hex.EncodeToString(key[:]), s: newServer(opt)}
	h.s.ready.Store(true)
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", h.serve)
	h.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go h.srv.Serve(ln)
	return h, nil
}

func (h *HTTP) authorized(r *http.Request) bool {
	got := r.Header.Get("Authorization")
	return subtle.ConstantTimeCompare([]byte(got), []byte("Bearer "+h.Token)) == 1
}

// local refuses a request whose Host is not loopback, so a web page cannot reach the server by DNS rebinding.
func local(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
	}
	return host == "127.0.0.1" || host == "localhost"
}

func (h *HTTP) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case !local(r):
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	case !h.authorized(r):
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	case r.Method != http.MethodPost:
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rep, ok := h.s.respond(raw)
	if !ok {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(rep)
}

func (h *HTTP) Close() error {
	return h.srv.Close()
}
