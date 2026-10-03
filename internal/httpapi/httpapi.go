package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/tseiman/HomeAuthMonitorGW/internal/cache"
)

type Options struct {
	Version             string
	HealthPublic        bool
	CertificateNotAfter func() time.Time
}
type API struct {
	store   *cache.Store
	token   func() string
	options Options
}

func New(store *cache.Store, token func() string, options Options) http.Handler {
	a := &API{store: store, token: token, options: options}
	return http.HandlerFunc(a.serve)
}
func (a *API) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !(a.options.HealthPublic && r.URL.Path == "/api/v1/health") && !validBearer(r.Header.Get("Authorization"), a.token()) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="automation-gateway"`)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch path {
	case "/api/v1/health":
		a.health(w)
		return
	case "/api/v1/sources":
		a.sources(w)
		return
	case "/api/v1/metrics":
		a.allMetrics(w)
		return
	case "/api/v1/quint/status":
		a.source(w, "quint", false, false)
		return
	case "/api/v1/quint/metrics":
		a.source(w, "quint", true, false)
		return
	case "/api/v1/wago/status":
		a.source(w, "wago", false, false)
		return
	case "/api/v1/wago/metrics":
		a.source(w, "wago", true, false)
		return
	case "/api/v1/wago/discovery":
		a.source(w, "wago", false, true)
		return
	}
	const prefix = "/api/v1/sources/"
	if strings.HasPrefix(path, prefix) {
		rest := strings.TrimPrefix(path, prefix)
		parts := strings.Split(rest, "/")
		if len(parts) == 1 && parts[0] != "" {
			a.source(w, parts[0], false, false)
			return
		}
		if len(parts) == 2 && parts[1] == "metrics" {
			a.source(w, parts[0], true, false)
			return
		}
		if len(parts) == 2 && parts[1] == "discovery" {
			a.source(w, parts[0], false, true)
			return
		}
	}
	writeError(w, http.StatusNotFound, "not found")
}
func validBearer(header, expected string) bool {
	const p = "Bearer "
	if !strings.HasPrefix(header, p) || expected == "" {
		return false
	}
	got := strings.TrimPrefix(header, p)
	a, b := sha256.Sum256([]byte(got)), sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}
func (a *API) health(w http.ResponseWriter) {
	status := "ok"
	code := http.StatusOK
	all := a.store.All()
	if len(all) == 0 {
		status = "failed"
		code = http.StatusServiceUnavailable
	} else {
		for _, s := range all {
			if !s.Available || s.Stale {
				status = "degraded"
				break
			}
		}
	}
	v := map[string]any{"status": status, "version": a.options.Version}
	if a.options.CertificateNotAfter != nil {
		v["certificate_not_after"] = a.options.CertificateNotAfter().UTC()
	}
	writeJSON(w, code, v)
}
func (a *API) sources(w http.ResponseWriter) {
	type summary struct {
		Name      string `json:"name"`
		Driver    string `json:"driver"`
		Available bool   `json:"available"`
		Stale     bool   `json:"stale"`
	}
	all := a.store.All()
	out := make([]summary, 0, len(all))
	for _, s := range all {
		out = append(out, summary{s.Name, s.Driver, s.Available, s.Stale})
	}
	writeJSON(w, 200, map[string]any{"sources": out})
}
func (a *API) source(w http.ResponseWriter, name string, metricsOnly, discovery bool) {
	s, ok := a.store.Snapshot(name)
	if !ok {
		writeError(w, 404, "source not found")
		return
	}
	if discovery {
		writeJSON(w, 200, map[string]any{"source": name, "discovery": s.Discovery})
		return
	}
	if metricsOnly {
		writeJSON(w, 200, map[string]any{"source": name, "available": s.Available, "stale": s.Stale, "last_attempt": s.LastAttempt, "last_success": s.LastSuccess, "error": s.Error, "metrics": s.Metrics})
		return
	}
	writeJSON(w, 200, s)
}
func (a *API) allMetrics(w http.ResponseWriter) {
	out := map[string]cache.Snapshot{}
	for _, s := range a.store.All() {
		out[s.Name] = s
	}
	writeJSON(w, 200, out)
}
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]string{"error": message})
}
