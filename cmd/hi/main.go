package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/wdw8276/hi/pkg/config"
	"github.com/wdw8276/hi/pkg/logx"
	"github.com/wdw8276/hi/pkg/proxy"
)

const usage = `hi — Claude Code multi-backend proxy

Usage:
  hi [command]

Commands:
  (bare)      Launch proxy + Claude Code (same as launch)
  launch      Start proxy and launch Claude Code
  proxy       Start proxy server only
  agent, cc   Attach Claude Code to an existing proxy
  status      Show configuration and proxy status
  init-config  Initialize ~/.hi/config.yaml (auto-detect from settings.json)
  statusline  Claude Code status line (model injection)
  version     Print version

Options:
  -b, --backend <name>   Backend to use (default: deepseek)
  -p, --port <port>      Proxy port (default: 18799)
  --log-level <level>    debug | info | warn | error (default: info)
  --log-file <path>      Write logs to file (default: ~/.hi/logs/hi.log)
  --preserve-statusline  Keep the existing statusLine command (don't replace with hi)

Examples:
  hi                                          # Launch proxy + Claude Code with Claude
  hi launch --backend claude --log-level debug # Launch with Claude, debug logging
  hi proxy --log-file /tmp/hi.log           # Proxy only, log to file
  hi agent --backend deepseek                 # Attach CC to existing proxy
  hi status                                    # Show config
`

var (
	logFile  string
	logLevel = "info"
	version  = "dev"
)

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "launch":
			initLogging()
			cmdLaunch()
		case "proxy":
			initLogging()
			cmdProxy()
		case "agent", "cc":
			initLogging()
			cmdAgent()
		case "status":
			cmdStatus()
		case "statusline":
			cmdStatusline()
		case "init-config":
			cmdInitConfig()
		case "version", "--version", "-V":
			fmt.Println("hi " + version)
		case "help", "--help", "-h":
			fmt.Print(usage)
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n%s", os.Args[1], usage)
			os.Exit(1)
		}
	} else {
		initLogging()
		cmdLaunch()
	}
}

func initLogging() {
	if logFile == "" {
		logFile = parseLogFile()
	}
	if lf := parseLogLevelFlag(); lf != "" {
		logLevel = lf
	}
	if logFile == "" {
		home, _ := os.UserHomeDir()
		logFile = filepath.Join(home, ".hi", "logs", "hi.log")
	}

	if err := logx.Init(logFile, logx.LevelStr(logLevel)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to init log file %s: %v\n", logFile, err)
	}
}

func cmdStatus() {
	cfgPath, _ := config.Path()
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Config:  %s\n", cfgPath)

	if !config.CCMissing() {
		home, _ := os.UserHomeDir()
		ccSettings := filepath.Join(home, ".claude", "settings.json")
		fmt.Printf("Claude:  detected at %s\n", ccSettings)
	}

	fmt.Printf("Proxy:   http://127.0.0.1:%d\n", cfg.ProxyPort)
	fmt.Printf("Active:  %s\n", cfg.ActiveBackend)
	if lf := logx.FilePath(); lf != "" {
		fmt.Printf("Log:     %s (level=%s)\n", lf, logLevel)
	}
	fmt.Println("Backends:")
	for name := range cfg.Backends {
		marker := " "
		if name == cfg.ActiveBackend {
			marker = "*"
		}
		fmt.Printf("  %s %s\n", marker, name)
	}
}

// cmdInitConfig initializes ~/.hi/config.yaml, auto-detecting values from
// ~/.claude/settings.json if present.
func cmdInitConfig() {
	cfgPath, _ := config.Path()

	// Check if config already exists.
	if _, err := os.Stat(cfgPath); err == nil {
		fmt.Fprintf(os.Stderr, "Config already exists at %s\n", cfgPath)
		fmt.Fprintf(os.Stderr, "Delete it first if you want to reinitialize: rm %s\n", cfgPath)
		os.Exit(1)
	}

	// Auto-detect from settings.json.
	ccSettingsPath := filepath.Join(mustHomeDir(), ".claude", "settings.json")
	baseURL := "https://api.anthropic.com"
	apiKeyRef := "${ANTHROPIC_API_KEY}"
	if data, err := os.ReadFile(ccSettingsPath); err == nil {
		var doc struct {
			Env map[string]string `json:"env"`
		}
		if json.Unmarshal(data, &doc) == nil {
			if u := doc.Env["ANTHROPIC_BASE_URL"]; u != "" {
				baseURL = u
			}
			if k := doc.Env["ANTHROPIC_API_KEY"]; k != "" {
				apiKeyRef = k
			}
		}
	}

	cfg := config.DefaultConfig()
	cfg.ActiveBackend = "deepseek"
	bc := cfg.Backends["claude"]
	bc.BaseURL = baseURL
	bc.APIKey = apiKeyRef
	cfg.Backends["claude"] = bc


	if err := cfg.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Initialized %s\n", cfgPath)
	fmt.Printf("  base_url: %s\n", baseURL)
	fmt.Printf("  api_key:  %s\n", maskKey(apiKeyRef))
	fmt.Println("You can now edit the file to customize backend models and pricing.")
}

func mustHomeDir() string {
	home, _ := os.UserHomeDir()
	return home
}

// cmdStatusline reads Claude Code status JSON from stdin, replaces the model
// with the hi proxy's active backend model, then delegates to the original
// status line script for the rest (cost, cache, balance).
func cmdStatusline() {
	stdinData, _ := io.ReadAll(os.Stdin)

	modelMap := map[string]string{
		"deepseek": "deepseek-v4-pro",
		"claude":   "claude-sonnet-4-6",
	}

	// Query hi proxy for current backend model.
	cfg, _ := config.Load()
	proxyPort := cfg.ProxyPort
	model := ""
	resp, err := httpGetImpl(fmt.Sprintf("http://127.0.0.1:%d/_proxy/status", proxyPort))
	if err == nil {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var status struct {
			ActiveBackend string `json:"active_backend"`
		}
		if json.Unmarshal(body, &status) == nil {
			if m, ok := modelMap[status.ActiveBackend]; ok {
				model = m
			}
		}
	}

	// Patch model in stdin JSON.
	if model != "" && len(stdinData) > 0 {
		var obj map[string]interface{}
		if json.Unmarshal(stdinData, &obj) == nil {
			if mod, ok := obj["model"].(map[string]interface{}); ok {
				mod["display_name"] = model
				mod["id"] = model
			}
			if patched, err := json.Marshal(obj); err == nil {
				stdinData = patched
			}
		}
	}
	// Delegate to the original status line script (auto-detected and saved
	// by CCOverride when it replaced statusLine.command with hi).
	realScript := config.OriginalStatusCommand()
	if realScript != "" {
		// Look up in PATH if it's a bare name.
		if !strings.Contains(realScript, "/") && !strings.Contains(realScript, "\\") {
			if p, err := exec.LookPath(realScript); err == nil {
				realScript = p
			}
		}
		logx.Debug("statusline: delegating to %s", realScript)

		cmd := exec.Command(realScript)
		cmd.Stdin = bytes.NewReader(stdinData)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			logx.Debug("statusline: delegate failed: %v", err)
		}
	}
}

func cmdProxy() {
	backend := parseBackend()
	cfg, err := config.Load()
	if err != nil {
		logx.Fatalf("Failed to load config: %v", err)
	}

	// Persist --backend flag to config so hi cc picks up the same default.
	if backend != "" && backend != cfg.ActiveBackend {
		cfg.ActiveBackend = backend
		if err := cfg.Save(); err != nil {
			logx.Warn("Failed to save active_backend: %v", err)
		}
	}

	logx.Info("hi %s — Claude Code multi-backend proxy", version)
	logx.Info("Config:   ~/.hi/config.yaml")
	logx.Info("Backends: claude, deepseek")
	logx.Info("Active:   %s", cfg.ActiveBackend)
	fmt.Println()

	// Generate slash commands so users can type /deepseek or /claude.
	if err := config.GenerateSlashCommands(cfg); err != nil {
		logx.Warn("failed to generate slash commands: %v", err)
	}

	fmt.Printf("hi: Proxy starting at http://127.0.0.1:%d (backend: %s)\n", cfg.ProxyPort, cfg.ActiveBackend)
	fmt.Printf("hi: Status:  curl -s http://127.0.0.1:%d/_proxy/status\n", cfg.ProxyPort)
	fmt.Printf("hi: Switch:  curl -sX POST http://127.0.0.1:%d/_proxy/mode -d 'backend=<name>'\n", cfg.ProxyPort)
	fmt.Println()

	defer logx.Close()
	if err := proxy.StartServer(cfg); err != nil {
		logx.Fatalf("Failed to start proxy: %v", err)
	}
}

func cmdLaunch() {
	backend := parseBackend()

	cfg, err := config.Load()
	if err != nil {
		logx.Fatalf("Failed to load config: %v", err)
	}

	if backend != "" && backend != cfg.ActiveBackend {
		cfg.ActiveBackend = backend
		if err := cfg.Save(); err != nil {
			logx.Warn("Warning: failed to save config: %v", err)
		}
	} else if backend == "" {
		backend = cfg.ActiveBackend
	}

	if _, ok := cfg.Backends[backend]; !ok {
		logx.Fatalf("Unknown backend: %s (available: claude, deepseek)", backend)
	}

	errCh, shutdown, err := proxy.StartServerInBackground(cfg)
	if err != nil {
		logx.Fatalf("Failed to start proxy: %v", err)
	}

	if proxyErr := <-errCh; proxyErr != nil {
		shutdown()
		fmt.Fprintf(os.Stderr, "hi: %v\n", proxyErr)
		return
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	defer func() {
		signal.Stop(sigCh)
		shutdown()
		logx.Close()
	}()

	cfgPath, _ := config.Path()
	fmt.Printf("\nhi: Claude Code starting with backend: %s\n", backend)
	fmt.Printf("hi: Config \u2192 %s\n", cfgPath)
	fmt.Printf("hi: Proxy  at http://127.0.0.1:%d\n", cfg.ProxyPort)
	fmt.Printf("hi: Switch backend: curl -sX POST http://127.0.0.1:%d/_proxy/mode -d 'backend=<name>'\n", cfg.ProxyPort)
	if lf := logx.FilePath(); lf != "" {
		fmt.Printf("hi: Logs → %s (level=%s)\n", lf, logLevel)
	}
	fmt.Println()

	// Generate slash commands so users can type /deepseek or /claude.
	if err := config.GenerateSlashCommands(cfg); err != nil {
		logx.Warn("failed to generate slash commands: %v", err)
	}

	claudeCmd := exec.Command("claude")
	claudeCmd.Stdin = os.Stdin
	claudeCmd.Stdout = os.Stdout
	claudeCmd.Stderr = os.Stderr

	apiKey := config.ResolveAPIKey(cfg.Backends[backend].APIKey)
	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.ProxyPort)

	// Temporarily patch ~/.claude/settings.json so Claude Code reads
	// hi's env vars instead of the persisted ones. ANTHROPIC_API_KEY
	// is intentionally left untouched — Claude Code needs it locally.
restoreCC, _ := config.CCOverride(map[string]string{
		"ANTHROPIC_BASE_URL":             proxyURL,
		"ANTHROPIC_AUTH_TOKEN":           apiKey,
		"ANTHROPIC_MODEL":                cfg.Backends[backend].Models.Sonnet,
		"ANTHROPIC_DEFAULT_OPUS_MODEL":   cfg.Backends[backend].Models.Opus,
		"ANTHROPIC_DEFAULT_SONNET_MODEL": cfg.Backends[backend].Models.Sonnet,
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":  cfg.Backends[backend].Models.Haiku,
	}, !preserveStatusline())
	defer restoreCC()

	logx.Info("Launching Claude Code with environment:")
	logx.Info("  ANTHROPIC_BASE_URL             = %s", proxyURL)
	logx.Info("  ANTHROPIC_AUTH_TOKEN           = %s", maskKey(apiKey))
	logx.Info("  ANTHROPIC_MODEL                = %s", cfg.Backends[backend].Models.Sonnet)
	logx.Info("  ANTHROPIC_DEFAULT_OPUS_MODEL   = %s", cfg.Backends[backend].Models.Opus)
	logx.Info("  ANTHROPIC_DEFAULT_SONNET_MODEL = %s", cfg.Backends[backend].Models.Sonnet)
	logx.Info("  ANTHROPIC_DEFAULT_HAIKU_MODEL  = %s", cfg.Backends[backend].Models.Haiku)
	logx.Info("  ANTHROPIC_API_KEY              = <kept from settings.json>")
	logx.Info("")

	env := os.Environ()
	// Strip inherited Anthropic vars FIRST, then append hi's own.
	env = filterEnv(env, "ANTHROPIC_BASE_URL")
	env = filterEnv(env, "ANTHROPIC_AUTH_TOKEN")
	env = filterEnv(env, "ANTHROPIC_MODEL")
	env = append(env,
		fmt.Sprintf("ANTHROPIC_BASE_URL=%s", proxyURL),
		fmt.Sprintf("ANTHROPIC_AUTH_TOKEN=%s", apiKey),
		fmt.Sprintf("ANTHROPIC_MODEL=%s", cfg.Backends[backend].Models.Sonnet),
		fmt.Sprintf("ANTHROPIC_DEFAULT_OPUS_MODEL=%s", cfg.Backends[backend].Models.Opus),
		fmt.Sprintf("ANTHROPIC_DEFAULT_SONNET_MODEL=%s", cfg.Backends[backend].Models.Sonnet),
		fmt.Sprintf("ANTHROPIC_DEFAULT_HAIKU_MODEL=%s", cfg.Backends[backend].Models.Haiku),
	)
	claudeCmd.Env = env

	go func() {
		<-sigCh
		if claudeCmd.Process != nil {
			claudeCmd.Process.Signal(syscall.SIGINT)
		}
	}()

	if err := claudeCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		logx.Fatalf("Failed to run Claude Code: %v", err)
	}

	fmt.Println("\nhi: Claude Code exited. Proxy stopped.")
}

// cmdAgent attaches a Claude Code agent to an already-running proxy.
func cmdAgent() {
	backend := parseBackend()
	cfg, err := config.Load()
	if err != nil {
		logx.Fatalf("Failed to load config: %v", err)
	}
	if backend == "" {
		backend = cfg.ActiveBackend
	}
	bc, ok := cfg.Backends[backend]
	if !ok {
		logx.Fatalf("Unknown backend: %s", backend)
	}

	apiKey := config.ResolveAPIKey(bc.APIKey)
	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.ProxyPort)

	fmt.Printf("\nhi: Claude Code agent starting (backend: %s, proxy: %s)\n", backend, proxyURL)
	fmt.Printf("hi: Hot-switch: curl -sX POST %s/_proxy/mode -d 'backend=<name>'\n", proxyURL)
	fmt.Println()

	// Patch settings.json temporarily so Claude Code reads the proxy address.
	// Settings.json env has higher priority than process env vars.
restoreCC, _ := config.CCOverride(map[string]string{
		"ANTHROPIC_BASE_URL":             proxyURL,
		"ANTHROPIC_AUTH_TOKEN":           apiKey,
		"ANTHROPIC_MODEL":                bc.Models.Sonnet,
		"ANTHROPIC_DEFAULT_OPUS_MODEL":   bc.Models.Opus,
		"ANTHROPIC_DEFAULT_SONNET_MODEL": bc.Models.Sonnet,
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":  bc.Models.Haiku,
	}, !preserveStatusline())
	defer restoreCC()

	// Generate slash commands so this agent has /deepseek /claude.
	config.GenerateSlashCommands(cfg)

	claudeCmd := exec.Command("claude")
	claudeCmd.Stdin = os.Stdin
	claudeCmd.Stdout = os.Stdout
	claudeCmd.Stderr = os.Stderr

	env := os.Environ()
	env = filterEnv(env, "ANTHROPIC_BASE_URL")
	env = filterEnv(env, "ANTHROPIC_AUTH_TOKEN")
	env = filterEnv(env, "ANTHROPIC_MODEL")
	env = append(env,
		fmt.Sprintf("ANTHROPIC_BASE_URL=%s", proxyURL),
		fmt.Sprintf("ANTHROPIC_AUTH_TOKEN=%s", apiKey),
		fmt.Sprintf("ANTHROPIC_MODEL=%s", bc.Models.Sonnet),
		fmt.Sprintf("ANTHROPIC_DEFAULT_OPUS_MODEL=%s", bc.Models.Opus),
		fmt.Sprintf("ANTHROPIC_DEFAULT_SONNET_MODEL=%s", bc.Models.Sonnet),
		fmt.Sprintf("ANTHROPIC_DEFAULT_HAIKU_MODEL=%s", bc.Models.Haiku),
	)
	claudeCmd.Env = env

	// Forward signals to Claude Code.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		if claudeCmd.Process != nil {
			claudeCmd.Process.Signal(syscall.SIGINT)
		}
	}()
	defer signal.Stop(sigCh)

	if err := claudeCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		logx.Fatalf("Failed to run Claude Code: %v", err)
	}
}

func parseBackend() string {
	for i, arg := range os.Args {
		if (arg == "--backend" || arg == "-b") && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}

func parseLogLevelFlag() string {
	for i, arg := range os.Args {
		if arg == "--log-level" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}

func preserveStatusline() bool {
	for _, a := range os.Args {
		if a == "--preserve-statusline" {
			return true
		}
	}
	return false
}

func parseLogFile() string {
	for i, arg := range os.Args {
		if arg == "--log-file" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}

func waitForProxy(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://127.0.0.1:%d/_proxy/status", port)

	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		resp, err := httpGet(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return true
			}
		}
	}
	return false
}

func httpGet(url string) (*httpResponse, error) {
	return httpGetImpl(url)
}

func maskKey(s string) string {
	if len(s) <= 8 {
		return "<empty>"
	}
	return "***" + s[len(s)-4:]
}

func filterEnv(env []string, key string) []string {
	prefix := key + "="
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}
