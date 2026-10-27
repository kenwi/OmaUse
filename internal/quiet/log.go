package quiet

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hakastein/gocldr/datetime"
	"golang.org/x/text/language"

	// CLDR date/time data for the locales we ship. language.Matcher maps
	// related tags (nb-NO, no, en-GB) onto these.
	_ "github.com/hakastein/gocldr/datetime/locales/en"
	_ "github.com/hakastein/gocldr/datetime/locales/nb"
	_ "github.com/hakastein/gocldr/datetime/locales/nn"
)

const logFileName = "quiet.log"

// DefaultLogPath is $XDG_STATE_HOME/omause/quiet.log (default ~/.local/state/omause).
func DefaultLogPath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, logFileName), nil
}

// Locales with linked CLDR data. Related BCP-47 tags match onto these.
var logLocales = []language.Tag{
	language.English,
	language.MustParse("nb"),
	language.MustParse("nn"),
}

var logLocaleMatcher = language.NewMatcher(logLocales)

var logHour12 = false

// Logger appends quiet-mode threshold lines when enabled.
type Logger struct {
	enabled bool
	path    string
	locale  string
	format  string
	now     func() time.Time
}

func newLogger(cfg LogSettings) Logger {
	locale := cfg.Locale
	if locale == "" {
		locale = "en"
	}
	format := cfg.Format
	if format == "" {
		format = "short"
	}
	return Logger{
		enabled: cfg.Enabled,
		path:    cfg.Path,
		locale:  locale,
		format:  format,
		now:     time.Now,
	}
}

func (l Logger) logThreshold(ev Event, scriptRan bool) error {
	if !l.enabled {
		return nil
	}
	msg := fmt.Sprintf("threshold %s %d%% (usage %d%%)",
		VolumeKey(ev.Volume), ev.Threshold.Percent, roundPct(ev.Volume))
	if ev.Threshold.Script != "" {
		if scriptRan {
			msg += "; script " + ev.Threshold.Script
		} else {
			msg += "; script failed:" + ev.Threshold.Script
		}
	}
	return l.append(msg)
}

func (l Logger) logReset(r Reset) error {
	if !l.enabled {
		return nil
	}
	msg := fmt.Sprintf("reset %s %d%% (usage %d%%)",
		VolumeKey(r.Volume), r.Percent, roundPct(r.Volume))
	return l.append(msg)
}

func (l Logger) append(msg string) error {
	path, err := expandLogPath(l.path)
	if err != nil {
		return err
	}
	if path == "" {
		path, err = DefaultLogPath()
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	line := formatLogTimestamp(l.now(), l.locale, l.format) + " " + msg + "\n"
	_, err = f.WriteString(line)
	return err
}

func expandLogPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

// resolveLogLocale parses a BCP-47 tag and maps it onto a locale with CLDR data.
// Empty becomes en.
func resolveLogLocale(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "en", nil
	}
	tag, err := language.Parse(s)
	if err != nil {
		return "", fmt.Errorf("invalid BCP-47 locale %q", s)
	}
	matched, _, _ := logLocaleMatcher.Match(tag)
	base, _ := matched.Base()
	return base.String(), nil
}

// resolveLogFormat accepts short (default) or long.
func resolveLogFormat(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "short":
		return "short", nil
	case "long":
		return "long", nil
	default:
		return "", fmt.Errorf("want short or long, got %q", s)
	}
}

func logTimestampOpts(format string) datetime.Options {
	dateStyle := "short"
	if format == "long" {
		dateStyle = "long"
	}
	// Medium time keeps seconds so fires and resets can be timed.
	return datetime.Options{
		DateStyle: dateStyle,
		TimeStyle: "medium",
		Hour12:    &logHour12,
	}
}

// formatLogTimestamp renders t with CLDR date/time for locale and format.
func formatLogTimestamp(t time.Time, locale, format string) string {
	return datetime.Format(locale, t.Local(), logTimestampOpts(format))
}
