package proxy

import (
	"encoding/json"
	"github.com/mars-base/hi/pkg/logx"
	"io"
	"net/http"
	"strings"
)

// handleControl dispatches /_proxy/* endpoints.
func (ps *ProxyState) handleControl(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/_proxy")
	path = strings.TrimSuffix(path, "/")

	switch {
	case (path == "/status" || path == "/status/") && r.Method == http.MethodGet:
		ps.handleStatus(w, r)
	case (path == "/cost" || path == "/cost/") && r.Method == http.MethodGet:
		ps.handleCost(w, r)
	case (path == "/mode" || path == "/mode/") && r.Method == http.MethodPost:
		ps.handleSwitchMode(w, r)
	case (path == "/mode" || path == "/mode/") && r.Method == http.MethodGet:
		ps.handleGetMode(w, r)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	}
}

// handleStatus returns current proxy state.
func (ps *ProxyState) handleStatus(w http.ResponseWriter, r *http.Request) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	names := make([]string, 0, len(ps.backends))
	for name := range ps.backends {
		names = append(names, name)
	}

	status := map[string]interface{}{
		"active_backend": ps.active,
		"uptime_seconds": int(ps.Uptime().Seconds()),
		"requests":       ps.RequestCount(),
		"backends":       names,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}

// handleCost returns current cost summary.
func (ps *ProxyState) handleCost(w http.ResponseWriter, r *http.Request) {
	summary := ps.costTracker.Summary()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(summary)
}

// handleGetMode returns the current backend mode.
func (ps *ProxyState) handleGetMode(w http.ResponseWriter, r *http.Request) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"mode": ps.active})
}

// handleSwitchMode switches the active backend.
func (ps *ProxyState) handleSwitchMode(w http.ResponseWriter, r *http.Request) {
	// Security: only allow localhost.
	origin := r.Header.Get("Origin")
	if origin != "" && !strings.HasPrefix(origin, "http://127.0.0.1") && !strings.HasPrefix(origin, "http://localhost") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "forbidden: only localhost can switch backends"})
		return
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1024))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to read body"})
		return
	}

	body := string(bodyBytes)
	// Support both "backend=name" and JSON {"backend": "name"}.
	var backendName string
	if idx := strings.Index(body, "backend="); idx >= 0 {
		backendName = body[idx+len("backend="):]
		// Trim any extra params.
		if space := strings.IndexByte(backendName, '&'); space >= 0 {
			backendName = backendName[:space]
		}
		backendName = strings.TrimSpace(backendName)
	} else {
		var req struct {
			Backend string `json:"backend"`
		}
		if err := json.Unmarshal(bodyBytes, &req); err == nil {
			backendName = req.Backend
		}
	}

	if backendName == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing backend parameter"})
		return
	}

	prev, err := ps.SwitchBackend(backendName)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	logx.Info("Control: backend switched %s → %s", prev, backendName)
	// Log env vars for the new backend.
	if b := ps.backends[backendName]; b != nil {
		info := b.ModelInfo()
		logx.Info("New backend env:")
		logx.Info("  ANTHROPIC_MODEL                = %s", info["sonnet"])
		logx.Info("  ANTHROPIC_DEFAULT_OPUS_MODEL   = %s", info["opus"])
		logx.Info("  ANTHROPIC_DEFAULT_SONNET_MODEL = %s", info["sonnet"])
		logx.Info("  ANTHROPIC_DEFAULT_HAIKU_MODEL  = %s", info["haiku"])
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"mode":     backendName,
		"previous": prev,
	})
}
