// Package api exposes the JSON API and serves the embedded frontend.
package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/andrinoff/server-health/internal/host"
)

// Config is the runtime wiring the dashboard needs: the machine sampler and
// the systemd units it may show and control.
type Config struct {
	Host     *host.Sampler
	Services []string // unit names, with or without the .service suffix
	Demo     bool     // serve invented readings and units instead of the machine
}

// Server holds the dependencies for all HTTP handlers.
type Server struct {
	static   fs.FS
	host     *host.Sampler
	services []string
	demo     bool
}

// NewServer builds the HTTP handler serving the API and the static frontend.
// A nil static fs is allowed (API-only, e.g. while developing the frontend).
func NewServer(static fs.FS, cfg Config) http.Handler {
	if cfg.Host == nil {
		cfg.Host = host.NewSampler(host.Options{})
	}
	s := &Server{static: static, host: cfg.Host, services: cleanUnits(cfg.Services), demo: cfg.Demo}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/host", s.getHost)
	mux.HandleFunc("GET /api/host/services", s.listServices)
	mux.HandleFunc("POST /api/host/services/{name}/{action}", s.controlService)
	mux.Handle("/", s.staticHandler())
	return logRequests(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Microsecond))
	})
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			log.Printf("write json: %v", err)
		}
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// staticHandler serves the embedded single-page frontend, falling back to
// index.html so deep links work. If the frontend was never built, it says how.
func (s *Server) staticHandler() http.Handler {
	if s.static == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			writeError(w, http.StatusServiceUnavailable, "frontend not built; run `make build`")
		})
	}
	fileServer := http.FileServerFS(s.static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" || p == "." {
			p = "index.html"
		}
		if _, err := fs.Stat(s.static, p); err != nil {
			if idx, readErr := fs.ReadFile(s.static, "index.html"); readErr == nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, string(idx))
				return
			}
		}
		fileServer.ServeHTTP(w, r)
	})
}
