package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wdw8276/hi/pkg/config"
	"github.com/wdw8276/hi/pkg/logx"
)

// backendTokens tracks token usage for a single backend.
type backendTokens struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	Requests     int64 `json:"requests"`
}

// persistedCost is the on-disk cost snapshot.
type persistedCost struct {
	Backends map[string]*backendTokens `json:"backends"`
}

// CostTracker tracks token usage and calculates costs.
type CostTracker struct {
	mu          sync.RWMutex
	usage       map[string]*backendTokens
	pricing     map[string]config.PricingPerMillion
	storagePath string
	stopCh      chan struct{}
}

// costFilePath returns the path to the persisted cost file.
func costFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".hi", ".cost.json"), nil
}

// NewCostTracker creates a new CostTracker with the given pricing.
// Loads previously persisted data and starts a background goroutine
// that flushes to disk every 30s.
func NewCostTracker(pricing map[string]config.PricingPerMillion) *CostTracker {
	ct := &CostTracker{
		usage:   make(map[string]*backendTokens),
		pricing: pricing,
		stopCh:  make(chan struct{}),
	}

	// Load persisted data.
	sp, err := costFilePath()
	if err == nil {
		ct.storagePath = sp
		data, err := os.ReadFile(sp)
		if err == nil {
			var pc persistedCost
			if json.Unmarshal(data, &pc) == nil && pc.Backends != nil {
				for k, v := range pc.Backends {
					ct.usage[k] = &backendTokens{
						InputTokens:  v.InputTokens,
						OutputTokens: v.OutputTokens,
						Requests:     v.Requests,
					}
				}
				logx.Info("cost: loaded persisted data (%d backends) from %s", len(pc.Backends), sp)
			}
		}
	}

	// Create file if missing.
	ct.saveLocked()

	// Periodic flush (every 30s) + final flush on Close.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				ct.mu.RLock()
				ct.saveLocked()
				ct.mu.RUnlock()
			case <-ct.stopCh:
				return
			}
		}
	}()

	return ct
}

// Record adds token usage for a backend (memory only, no disk write).
func (ct *CostTracker) Record(backend string, inputTokens, outputTokens int64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if _, ok := ct.usage[backend]; !ok {
		ct.usage[backend] = &backendTokens{}
	}
	ct.usage[backend].InputTokens += inputTokens
	ct.usage[backend].OutputTokens += outputTokens
	ct.usage[backend].Requests++
}

// Close stops the periodic flush and performs a final save.
func (ct *CostTracker) Close() {
	close(ct.stopCh)
	ct.mu.Lock()
	ct.saveLocked()
	ct.mu.Unlock()
	logx.Info("cost: final flush complete")
}

// Flush writes current usage to disk immediately.
func (ct *CostTracker) Flush() {
	ct.mu.RLock()
	ct.saveLocked()
	ct.mu.RUnlock()
}

// saveLocked writes current usage to disk. Caller must hold at least ct.mu.RLock.
func (ct *CostTracker) saveLocked() {
	if ct.storagePath == "" {
		return
	}
	pc := persistedCost{Backends: ct.usage}
	data, err := json.Marshal(pc)
	if err != nil {
		logx.Warn("cost: marshal error: %v", err)
		return
	}
	dir := filepath.Dir(ct.storagePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logx.Warn("cost: mkdir error: %v", err)
		return
	}
	if err := os.WriteFile(ct.storagePath, data, 0600); err != nil {
		logx.Warn("cost: write error: %v", err)
		return
	}
	logx.Debug("cost: flushed to %s (%d backends)", ct.storagePath, len(ct.usage))
}

// CostSummary holds the computed cost breakdown.
type CostSummary struct {
	Backends            map[string]BackendCost `json:"backends"`
	TotalCost           float64                `json:"total_cost"`
	AnthropicEquivalent float64                `json:"anthropic_equivalent"`
	Savings             float64                `json:"savings"`
}

// BackendCost is the per-backend cost breakdown.
type BackendCost struct {
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	Requests            int64   `json:"requests"`
	Cost                float64 `json:"cost"`
	AnthropicEquivalent float64 `json:"anthropic_equivalent"`
}

// Summary computes the current cost summary.
func (ct *CostTracker) Summary() CostSummary {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	summary := CostSummary{
		Backends: make(map[string]BackendCost, len(ct.usage)),
	}

	anthropicPrice := ct.pricing["claude"]

	for name, tokens := range ct.usage {
		price := ct.pricing[name]
		if _, ok := ct.pricing[name]; !ok {
			price = ct.pricing["_default"]
		}

		cost := (float64(tokens.InputTokens)*price.Input + float64(tokens.OutputTokens)*price.Output) / 1_000_000
		anthropicEq := (float64(tokens.InputTokens)*anthropicPrice.Input + float64(tokens.OutputTokens)*anthropicPrice.Output) / 1_000_000

		summary.Backends[name] = BackendCost{
			InputTokens:         tokens.InputTokens,
			OutputTokens:        tokens.OutputTokens,
			Requests:            tokens.Requests,
			Cost:                roundTo(cost, 4),
			AnthropicEquivalent: roundTo(anthropicEq, 4),
		}
		summary.TotalCost += cost
		summary.AnthropicEquivalent += anthropicEq
	}

	summary.TotalCost = roundTo(summary.TotalCost, 4)
	summary.AnthropicEquivalent = roundTo(summary.AnthropicEquivalent, 4)
	summary.Savings = roundTo(summary.AnthropicEquivalent-summary.TotalCost, 4)

	return summary
}

func roundTo(v float64, decimals int) float64 {
	pow := 1.0
	for i := 0; i < decimals; i++ {
		pow *= 10
	}
	return float64(int(v*pow+0.5)) / pow
}
