// Package admin is the HTTP server every Go service exposes next to its main
// port: /healthz (liveness: the process is up), /readyz (readiness: its
// dependencies answer and it is not shutting down) and /version (spec 6.5,
// 9.1, 9.3). Metrics will be added here in stage 4.
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/sokolovlev/polka/libs/go/buildinfo"
	"github.com/sokolovlev/polka/libs/go/lifecycle"
)

// Check reports whether one dependency (database, Kafka, ...) is usable.
type Check struct {
	Name  string
	Check func(ctx context.Context) error
}

const checkTimeout = 2 * time.Second

// Handler returns the admin routes. Liveness never checks dependencies: a
// broken database must not make Kubernetes restart every pod.
func Handler(info buildinfo.Info, ready *lifecycle.Readiness, checks ...Check) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if ready != nil && ready.Draining() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "draining"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
		defer cancel()
		failed := map[string]string{}
		for _, c := range checks {
			if err := c.Check(ctx); err != nil {
				failed[c.Name] = err.Error()
			}
		}
		if len(failed) > 0 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "unavailable", "failed": failed})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, info)
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// HTTPServer adapts *http.Server to lifecycle.Component.
type HTTPServer struct {
	name string
	srv  *http.Server
	ln   net.Listener
}

// NewHTTPServer binds addr immediately, so a busy port fails at startup.
func NewHTTPServer(ctx context.Context, name, addr string, h http.Handler) (*HTTPServer, error) {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}
	return NewHTTPServerFromListener(name, ln, h), nil
}

// NewHTTPServerFromListener serves on an already bound listener.
func NewHTTPServerFromListener(name string, ln net.Listener, h http.Handler) *HTTPServer {
	return &HTTPServer{
		name: name,
		ln:   ln,
		srv: &http.Server{
			Handler:           h,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

// Addr is the bound address (useful with ":0" in tests).
func (s *HTTPServer) Addr() string { return s.ln.Addr().String() }

// Name implements lifecycle.Component.
func (s *HTTPServer) Name() string { return s.name }

// Run implements lifecycle.Component.
func (s *HTTPServer) Run(context.Context) error {
	if err := s.srv.Serve(s.ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

// Shutdown stops accepting connections and waits for active requests.
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	if err := s.srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}
	return nil
}
