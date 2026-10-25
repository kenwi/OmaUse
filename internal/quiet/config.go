package quiet

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const configFileName = "config.json"

// Threshold is a use-percent level that can notify and optionally run a script.
type Threshold struct {
	Percent int    `json:"percent"`
	Script  string `json:"script,omitempty"`
}

// Config is the quiet-mode alert settings under ~/.config/omause.
type Config struct {
	// Enabled turns threshold alerts on or off. Omitted means enabled.
	Enabled    *bool       `json:"enabled"`
	Thresholds []Threshold `json:"thresholds"`
}

// AlertsEnabled is true when threshold checks should run.
func (c Config) AlertsEnabled() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// ConfigPath is $XDG_CONFIG_HOME/omause/config.json (default ~/.config/omause).
func ConfigPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

func configDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "omause"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "omause"), nil
}

// LoadConfig reads the quiet-mode config.
// Missing file returns an empty config and false for ok.
func LoadConfig() (Config, bool, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, false, nil
		}
		return Config{}, false, err
	}
	cfg, err := parseConfig(data)
	if err != nil {
		return Config{}, true, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, true, nil
}

func parseConfig(data []byte) (Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	cfg.normalize()
	return cfg, nil
}

func (c *Config) validate() error {
	for i, t := range c.Thresholds {
		if t.Percent < 1 || t.Percent > 100 {
			return fmt.Errorf("thresholds[%d]: percent must be 1-100, got %d", i, t.Percent)
		}
	}
	return nil
}

// normalize sorts thresholds ascending and keeps one entry per percent
// (later definitions win, so a script can replace a bare percent).
func (c *Config) normalize() {
	byPct := map[int]Threshold{}
	order := make([]int, 0)
	for _, t := range c.Thresholds {
		if _, ok := byPct[t.Percent]; !ok {
			order = append(order, t.Percent)
		}
		byPct[t.Percent] = t
	}
	sort.Ints(order)
	out := make([]Threshold, 0, len(order))
	for _, pct := range order {
		out = append(out, byPct[pct])
	}
	c.Thresholds = out
}
