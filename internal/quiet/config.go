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

// FluxSettings controls phone alerts via `flux-cli notify` (or `flux notify`).
// JSON may be a bool ("flux": true) or an object:
//
//	"flux": { "enabled": true, "announce-hostname": true }
type FluxSettings struct {
	Enabled          bool `json:"enabled"`
	AnnounceHostname bool `json:"announce-hostname"`
}

// UnmarshalJSON accepts a bool or an object for backward compatibility.
func (f *FluxSettings) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	switch s {
	case "true":
		*f = FluxSettings{Enabled: true}
		return nil
	case "false", "null", "":
		*f = FluxSettings{}
		return nil
	}
	var obj struct {
		Enabled          *bool `json:"enabled"`
		AnnounceHostname bool  `json:"announce-hostname"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("flux: want bool or object, got %s", s)
	}
	if obj.Enabled != nil {
		f.Enabled = *obj.Enabled
	} else {
		// Object form without enabled implies flux is on.
		f.Enabled = true
	}
	f.AnnounceHostname = obj.AnnounceHostname
	return nil
}

// LogSettings controls quiet-mode threshold logging.
// JSON may be a bool ("log": true) or an object:
//
//	"log": { "enabled": true, "path": "...", "locale": "nb", "format": "long" }
//
// Path defaults to ~/.local/state/omause/quiet.log. Locale is a BCP-47
// language tag (for example "en", "nb", "nb-NO"); English is the default.
// Format is "short" (default) or "long" for the CLDR timestamp style.
type LogSettings struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path,omitempty"`
	Locale  string `json:"locale,omitempty"`
	Format  string `json:"format,omitempty"`
}

// UnmarshalJSON accepts a bool or an object for log settings.
func (l *LogSettings) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	switch s {
	case "true":
		*l = LogSettings{Enabled: true}
		return nil
	case "false", "null", "":
		*l = LogSettings{}
		return nil
	}
	var obj struct {
		Enabled *bool  `json:"enabled"`
		Path    string `json:"path"`
		Locale  string `json:"locale"`
		Format  string `json:"format"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("log: want bool or object, got %s", s)
	}
	if obj.Enabled != nil {
		l.Enabled = *obj.Enabled
	} else {
		// Object form without enabled implies logging is on.
		l.Enabled = true
	}
	l.Path = strings.TrimSpace(obj.Path)
	l.Locale = strings.TrimSpace(obj.Locale)
	l.Format = strings.TrimSpace(obj.Format)
	return nil
}

// Config is the quiet-mode alert settings under ~/.config/omause.
type Config struct {
	// Enabled turns threshold alerts on or off. Omitted means enabled.
	Enabled *bool        `json:"enabled"`
	Flux    FluxSettings `json:"flux"`
	Log     LogSettings  `json:"log"`
	// Ignore lists mount paths that never alert. A trailing * matches that
	// path and anything mounted under it, for example "/run/media/*".
	Ignore     []string    `json:"ignore,omitempty"`
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
	if _, err := resolveLogLocale(c.Log.Locale); err != nil {
		return fmt.Errorf("log.locale: %w", err)
	}
	if _, err := resolveLogFormat(c.Log.Format); err != nil {
		return fmt.Errorf("log.format: %w", err)
	}
	for i, p := range c.Ignore {
		if err := validateIgnore(p); err != nil {
			return fmt.Errorf("ignore[%d]: %w", i, err)
		}
	}
	for i, t := range c.Thresholds {
		if t.Percent < 1 || t.Percent > 100 {
			return fmt.Errorf("thresholds[%d]: percent must be 1-100, got %d", i, t.Percent)
		}
	}
	return nil
}

// validateIgnore rejects a blank pattern and a star that would match every mount.
func validateIgnore(pattern string) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return fmt.Errorf("path is empty")
	}
	body := pattern
	if strings.HasSuffix(body, "*") {
		body = strings.TrimSuffix(body, "*")
	}
	body = normalizeDrive(body)
	if body == "" || body == "/" {
		return fmt.Errorf("%q matches every mount", pattern)
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
	c.Ignore = normalizeIgnore(c.Ignore)
	c.Log.Locale, _ = resolveLogLocale(c.Log.Locale)
	c.Log.Format, _ = resolveLogFormat(c.Log.Format)
	if c.Log.Enabled && c.Log.Path == "" {
		if p, err := DefaultLogPath(); err == nil {
			c.Log.Path = p
		}
	}
}

// normalizeIgnore trims, canonicalizes, and drops duplicate ignore patterns.
// A trailing * is stored as "<path>/*".
func normalizeIgnore(patterns []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(patterns))
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasSuffix(p, "*") {
			prefix := normalizeDrive(strings.TrimSuffix(p, "*"))
			p = prefix + "/*"
		} else {
			p = normalizeDrive(p)
		}
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// ignoredPath reports whether a mount path matches an ignore pattern.
// "/run/media/*" matches "/run/media" and any path under it.
func ignoredPath(where string, patterns []string) bool {
	where = normalizeDrive(where)
	if where == "" {
		return false
	}
	for _, p := range patterns {
		if prefix, ok := strings.CutSuffix(p, "/*"); ok {
			if where == prefix || strings.HasPrefix(where, prefix+"/") {
				return true
			}
			continue
		}
		if where == p {
			return true
		}
	}
	return false
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
