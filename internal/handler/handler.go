// Package handler exposes the webhook endpoints and serves the built site.
package handler

import (
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mojro/docs-platform/internal/builder"
)

// Config controls which pushes are allowed to publish.
type Config struct {
	Secret  string
	SiteDir string
	// PublishFeatureBranches allows branches outside the protected list to be
	// published as their own version. Turn this off to restrict the site to
	// long-lived branches only.
	PublishFeatureBranches bool
}

type Handler struct {
	b   *builder.Builder
	cfg Config

	// Builds run one at a time. Requests arriving during a build are coalesced
	// per (service, branch) so a burst of pushes does not queue N builds.
	mu       sync.Mutex
	building bool
	pending  map[buildKey]string // key -> cloneURL
}

type buildKey struct {
	service string
	branch  string
}

func New(b *builder.Builder, cfg Config) *Handler {
	return &Handler{b: b, cfg: cfg, pending: map[buildKey]string{}}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.healthz)
	mux.HandleFunc("/readyz", h.readyz)
	mux.HandleFunc("/api/services", h.services)
	mux.HandleFunc("/webhook/{service}", h.webhook)
	mux.Handle("/", h.site())
	return mux
}

func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readyz reports unready until a site exists, so the Gateway doesn't route to a
// pod that would only serve errors.
func (h *Handler) readyz(w http.ResponseWriter, r *http.Request) {
	if _, err := os.Stat(filepath.Join(h.cfg.SiteDir, "index.html")); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "no site built yet",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handler) services(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.b.Services())
}

// site serves the rendered static HTML directly — no nginx sidecar needed.
func (h *Handler) site() http.Handler {
	fs := http.FileServer(http.Dir(h.cfg.SiteDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(filepath.Join(h.cfg.SiteDir, "index.html")); err != nil {
			http.Error(w, "docs not built yet", http.StatusServiceUnavailable)
			return
		}
		// versions.json changes on every build; never let it be cached.
		if filepath.Base(r.URL.Path) == "versions.json" {
			w.Header().Set("Cache-Control", "no-store")
		}
		// Never expose a raw directory listing: a directory without its own
		// index.html should 404 rather than enumerate the tree.
		if strings.HasSuffix(r.URL.Path, "/") {
			candidate := filepath.Join(h.cfg.SiteDir, filepath.Clean(r.URL.Path), "index.html")
			if _, err := os.Stat(candidate); err != nil {
				http.NotFound(w, r)
				return
			}
		}
		fs.ServeHTTP(w, r)
	})
}

// webhook handles POST /webhook/<service>?token=…&clone_url=…&ref=…
func (h *Handler) webhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if subtle.ConstantTimeCompare(
		[]byte(r.URL.Query().Get("token")), []byte(h.cfg.Secret)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	service := r.PathValue("service")
	if !builder.ValidServiceName(service) {
		http.Error(w, "service must be lowercase alphanumeric with dashes", http.StatusBadRequest)
		return
	}

	cloneURL := r.URL.Query().Get("clone_url")
	if cloneURL == "" {
		http.Error(w, "missing clone_url", http.StatusBadRequest)
		return
	}

	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = refFromBitbucket(r)
	}
	if !builder.ValidBranchName(ref) {
		http.Error(w, "invalid ref", http.StatusBadRequest)
		return
	}

	// A push to a feature branch is normal traffic when publishing is limited
	// to long-lived branches, so acknowledge with 200 rather than an error that
	// Bitbucket would retry.
	if !h.b.IsProtected(ref) && !h.cfg.PublishFeatureBranches {
		log.Printf("skipping %s@%s: feature branch publishing disabled", service, ref)
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "skipped: feature branch publishing disabled",
			"service": service,
			"ref":     ref,
		})
		return
	}

	h.enqueue(service, cloneURL, ref)
	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "build queued",
		"service": service,
		"ref":     ref,
	})
}

func (h *Handler) enqueue(service, cloneURL, ref string) {
	key := buildKey{service: service, branch: ref}

	h.mu.Lock()
	h.pending[key] = cloneURL
	if h.building {
		h.mu.Unlock()
		return
	}
	h.building = true
	h.mu.Unlock()

	go h.drain()
}

// drain builds every pending (service, branch) until none remain.
func (h *Handler) drain() {
	for {
		h.mu.Lock()
		var key buildKey
		var url string
		found := false
		for k, v := range h.pending {
			key, url, found = k, v, true
			break
		}
		if !found {
			h.building = false
			h.mu.Unlock()
			return
		}
		delete(h.pending, key)
		h.mu.Unlock()

		if err := h.b.Build(key.service, url, key.branch); err != nil {
			log.Printf("build %s@%s failed: %v", key.service, key.branch, err)
		} else {
			log.Printf("build %s@%s ok", key.service, key.branch)
		}
	}
}

// refFromBitbucket pulls the pushed branch out of a Bitbucket webhook body,
// falling back to "main" when the payload isn't a recognizable push event.
func refFromBitbucket(r *http.Request) string {
	var payload struct {
		Push struct {
			Changes []struct {
				New struct {
					Name string `json:"name"`
					Type string `json:"type"`
				} `json:"new"`
			} `json:"changes"`
		} `json:"push"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
		for _, c := range payload.Push.Changes {
			// Ignore tag pushes and branch deletions.
			if c.New.Name != "" && (c.New.Type == "" || c.New.Type == "branch") {
				return c.New.Name
			}
		}
	}
	return "main"
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
