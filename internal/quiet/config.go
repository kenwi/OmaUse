package quiet

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const configFileName = "config.json"

// Threshold is a use-percent level that can notify and optionally run a script.
// Drive, when set, limits the threshold to that volume (mount path or name).
type Threshold struct {
	Percent int    `json:"percent"`
	Drive   string `json:"drive,omitempty"`
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

type thresholdKey struct {
	pct   int
	drive string
}

// normalize sorts thresholds by percent then drive, and keeps one entry per
// (percent, drive) pair (later definitions win).
func (c *Config) normalize() {
	byKey := map[thresholdKey]Threshold{}
	order := make([]thresholdKey, 0)
	for _, t := range c.Thresholds {
		t.Drive = normalizeDrive(t.Drive)
		k := thresholdKey{t.Percent, t.Drive}
		if _, ok := byKey[k]; !ok {
			order = append(order, k)
		}
		byKey[k] = t
	}
	sort.SliceStable(order, func(i, j int) bool {
		if order[i].pct != order[j].pct {
			return order[i].pct < order[j].pct
		}
		return order[i].drive < order[j].drive
	})
	out := make([]Threshold, 0, len(order))
	for _, k := range order {
		out = append(out, byKey[k])
	}
	c.Thresholds = out
}

func normalizeDrive(drive string) string {
	drive = strings.TrimSpace(drive)
	if drive == "" || drive == "-" {
		return ""
	}
	if drive != "/" {
		drive = strings.TrimRight(drive, "/")
	}
	return drive
}
