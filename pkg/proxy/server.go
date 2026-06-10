package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mars-base/hi/pkg/config"
	"github.com/mars-base/hi/pkg/logx"
)

// ProxyState holds the running proxy's mutable state.
type ProxyState struct {
	mu sync.RWMutex

	// active is the name of the currently active backend.
	active string

	// backends holds all registered backends by name.
	backends map[string]Backend

	// fallback is the Anthropic fallback URL for non-model requests.
	fallback *url.URL

	// costTracker tracks token usage and costs per backend.
	costTracker *CostTracker

	// hadNonAnthropic tracks whether we've used a non-Anthropic backend.
	hadNonAnthropic atomic.Bool

	// startTime records when the proxy started.
	startTime time.Time

	// reqCount counts total proxied requests.
	reqCount atomic.Uint64
}

// NewProxyState creates a new ProxyState from config.
func NewProxyState(cfg *config.Config) (*ProxyState, error) {
	backends := make(map[string]Backend, len(cfg.Backends))
	for name, bc := range cfg.Backends {
		b, err := NewBackend(name, bc)
		if err != nil {
			return nil, fmt.Errorf("failed to create backend %s: %w", name, err)
		}
		backends[name] = b
	}

	if _, ok := backends[cfg.ActiveBackend]; !ok {
		return nil, fmt.Errorf("active backend %q not found in config", cfg.ActiveBackend)
	}

	fallback, _ := url.Parse("https://api.anthropic.com")

	ps := &ProxyState{
		active:      cfg.ActiveBackend,
		backends:    backends,
		fallback:    fallback,
		costTracker: NewCostTracker(cfg.GetPricing()),
		startTime:   time.Now(),
	}
	// Seed request counter from persisted cost data so it survives restarts.
	ps.reqCount.Store(uint64(ps.costTracker.TotalRequests()))

	return ps, nil
}

// ActiveBackend returns the currently active backend.
func (ps *ProxyState) ActiveBackend() Backend {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.backends[ps.active]
}

// ActiveName returns the currently active backend name.
func (ps *ProxyState) ActiveName() string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.active
}

// SwitchBackend switches to the named backend. Returns the previous name.
func (ps *ProxyState) SwitchBackend(name string) (string, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if name == "anthropic" {
		prev := ps.active
		ps.active = "claude"
		return prev, nil
	}

	b, ok := ps.backends[name]
	if !ok {
		return "", fmt.Errorf("unknown backend: %s (valid: %s)", name, ps.backendNames())
	}

	prev := ps.active
	ps.active = name

	if b.Name() != "claude" {
		ps.hadNonAnthropic.Store(true)
	}

	logx.Info("Backend switched: %s → %s", prev, name)
	return prev, nil
}

// HadNonAnthropic returns true if any non-Claude backend has been used.
func (ps *ProxyState) HadNonAnthropic() bool {
	return ps.hadNonAnthropic.Load()
}

// Uptime returns the duration since the proxy started.
func (ps *ProxyState) Uptime() time.Duration {
	return time.Since(ps.startTime)
}

// RequestCount returns the total number of proxied requests.
func (ps *ProxyState) RequestCount() uint64 {
	return ps.reqCount.Load()
}

func (ps *ProxyState) incrRequestCount() {
	ps.reqCount.Add(1)
}

func (ps *ProxyState) backendNames() string {
	names := make([]string, 0, len(ps.backends))
	for n := range ps.backends {
		names = append(names, n)
	}
	return stringsJoin(names, ", ")
}

// Close shuts down the cost tracker and flushes final data to disk.
func (ps *ProxyState) Close() {
	ps.costTracker.Close()
}

func stringsJoin(s []string, sep string) string {
	if len(s) == 0 {
		return ""
	}
	r := s[0]
	for i := 1; i < len(s); i++ {
		r += sep + s[i]
	}
	return r
}

// ServeHTTP handles incoming HTTP requests — routing model calls to the
// active backend and everything else to Anthropic.
func (ps *ProxyState) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Handle control endpoints.
	if stringsHasPrefix(path, "/_proxy/") {
		ps.handleControl(w, r)
		return
	}

	ps.incrRequestCount()

	isModel := IsModelRequest(path)
	activeName := ps.ActiveName()
	backend := ps.ActiveBackend()

	// Route ALL requests through the active backend.
	// This ensures /login, auth validation etc. also hit the same upstream.
	// Model-specific transforms (name remap, thinking strip) are only applied
	// when isModel is true in transformRequestBody.
	dest := backend.TargetURL()
	authBackend := backend

	// Build the upstream URL.
	upstreamURL := *dest
	upstreamURL.Path = joinURLPath(dest.Path, path)
	upstreamURL.RawQuery = r.URL.RawQuery

	// Read the request body.
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	// Transform the request body if needed.
	transformed := ps.transformRequestBody(bodyBytes, isModel, activeName, backend)

	// Create the upstream request.
	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL.String(), nil)
	if err != nil {
		http.Error(w, "Failed to create upstream request", http.StatusInternalServerError)
		return
	}

	// Copy headers from the original request.
	for key, values := range r.Header {
		for _, v := range values {
			upstreamReq.Header.Add(key, v)
		}
	}
	cleanRequestHeaders(upstreamReq.Header)
	upstreamReq.Header.Set("Host", dest.Host)

	// Strip original auth and inject backend auth for all requests.
	// Everything routes through the active backend now.
	upstreamReq.Header.Del("Authorization")
	upstreamReq.Header.Del("x-api-key")
	if authBackend != nil {
		authBackend.SetAuth(upstreamReq)
	}

	// Set the request body.
	upstreamReq.Body = io.NopCloser(strings.NewReader(string(transformed)))
	upstreamReq.ContentLength = int64(len(transformed))
	upstreamReq.Header.Set("Content-Length", strconv.Itoa(len(transformed)))

	startTime := time.Now()

	resp, err := newTransport().RoundTrip(upstreamReq)
	if err != nil {
		logx.Error("Upstream error for %s: %v", activeName, err)
		http.Error(w, `{"error":{"message":"upstream connection error"}}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	elapsed := time.Since(startTime)
	logx.Info("#%d %s %s %s %d %s", ps.RequestCount(), r.Method, backend.Name(), path, resp.StatusCode, elapsed.Round(time.Millisecond))
	logx.Debug("  -> upstream: %s %s", r.Method, upstreamURL.String())
	logx.Debug("  <- status=%d content-type=%s", resp.StatusCode, resp.Header.Get("Content-Type"))

	// Log request body on non-2xx responses for diagnosis.
	if resp.StatusCode >= 400 {
		bodyStr := string(transformed)
		if len(bodyStr) > 2000 {
			head := bodyStr[:500]
			tail := bodyStr[len(bodyStr)-1000:]
			logx.Warn("  req body head (status=%d): %s", resp.StatusCode, head)
			logx.Warn("  req body tail (status=%d): %s", resp.StatusCode, tail)
		} else {
			logx.Warn("  req body (status=%d): %s", resp.StatusCode, bodyStr)
		}
	}

	// Process the response (SSE normalization, cost tracking).
	ps.processResponse(w, r, resp, backend)
}

// StartServer starts the HTTP proxy server and blocks.
func StartServer(cfg *config.Config) error {
	state, err := NewProxyState(cfg)
	if err != nil {
		return fmt.Errorf("failed to create proxy state: %w", err)
	}
	defer state.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.ProxyPort)

	// Pre-acquire listener to detect port conflicts before printing startup banner.
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
	if err != nil {
		return fmt.Errorf("port %s already in use — another hi proxy is running. Use 'hi cc' or 'hi agent' to attach instead", addr)
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      state,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 10 * time.Minute, // SSE streams can be long.
		IdleTimeout:  120 * time.Second,
	}

	logx.Info("Proxy listening on %s (active: %s)", addr, state.ActiveName())
	logx.Info("Control: curl -s http://%s/_proxy/status", addr)
	logx.Info("Switch:  curl -sX POST http://%s/_proxy/mode -d 'backend=deepseek'", addr)

	fmt.Printf("hi: Proxy started at http://%s (backend: %s)\n", addr, state.ActiveName())
	fmt.Printf("hi: Status:  curl -s http://%s/_proxy/status\n", addr)
	fmt.Printf("hi: Switch:  curl -sX POST http://%s/_proxy/mode -d 'backend=<name>'\n", addr)
	fmt.Println()

	// Graceful shutdown on SIGINT / SIGTERM.
	idleConns := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		signal.Stop(sigCh)
		logx.Info("Proxy shutting down...")
		srv.Shutdown(context.Background())
		close(idleConns)
	}()

	err = srv.Serve(ln)
	if err != nil && err != http.ErrServerClosed {
		if strings.Contains(err.Error(), "address already in use") {
			port := cfg.ProxyPort
			return fmt.Errorf("%s", portInUseMsg(port))
		}
		return err
	}
	<-idleConns
	return nil
}

// StartServerInBackground starts the proxy in a goroutine and returns
// a channel that receives any startup error, plus a shutdown function.
func StartServerInBackground(cfg *config.Config) (<-chan error, func(), error) {
	state, err := NewProxyState(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create proxy state: %w", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.ProxyPort)

	// Acquire the listener first so port-in-use is detected immediately,
	// before we risk a false-positive from polling another proxy's endpoint.
	ln, listenErr := (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
	ready := make(chan error, 1)

	if listenErr != nil {
		ready <- fmt.Errorf("%s", portInUseMsg(cfg.ProxyPort))
		shutdown := func() {}
		return ready, shutdown, nil
	}

	srv := &http.Server{
		Handler:      state,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logx.Info("Proxy listening on %s (active: %s)", addr, state.ActiveName())
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			ready <- err
		}
	}()
	go func() { ready <- nil }()

	shutdown := func() {
		state.Close()
		srv.Shutdown(context.Background())
	}

	return ready, shutdown, nil
}

func portInUseMsg(port int) string {
	return fmt.Sprintf("port :%d already in use — another hi proxy is running. Use 'hi cc' or 'hi agent' to attach instead", port)
}

func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func joinURLPath(base, rest string) string {
	// Strip trailing slash from base.
	for len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	// Strip leading slash from rest.
	for len(rest) > 0 && rest[0] == '/' {
		rest = rest[1:]
	}
	return base + "/" + rest
}
