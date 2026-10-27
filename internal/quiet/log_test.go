package quiet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"usage/internal/usage"
)

func TestFormatLogTimestamp(t *testing.T) {
	ts := time.Date(2026, 10, 27, 17, 49, 0, 0, time.Local)
	enShort := formatLogTimestamp(ts, "en", "short")
	if enShort != "10/27/26, 17:49:00" {
		t.Fatalf("en short=%q", enShort)
	}
	enLong := formatLogTimestamp(ts, "en", "long")
	if enLong != "October 27, 2026 at 17:49:00" {
		t.Fatalf("en long=%q", enLong)
	}
	nbShort := formatLogTimestamp(ts, "nb", "short")
	if nbShort != "27.10.2026, 17:49:00" {
		t.Fatalf("nb short=%q", nbShort)
	}
	nbLong := formatLogTimestamp(ts, "nb", "long")
	if nbLong != "27. oktober 2026 kl. 17:49:00" {
		t.Fatalf("nb long=%q", nbLong)
	}
}

func TestResolveLogLocale(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "en"},
		{"en", "en"},
		{"en-GB", "en"},
		{"nb", "nb"},
		{"nb-NO", "nb"},
		{"no", "nb"},
		{"nn", "nn"},
		{"sv", "en"}, // no Swedish CLDR linked; matcher falls back
	}
	for _, tc := range cases {
		got, err := resolveLogLocale(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}
	if _, err := resolveLogLocale("norwegian"); err == nil {
		t.Fatal("expected error for non BCP-47 tag")
	}
}

func TestResolveLogFormat(t *testing.T) {
	got, err := resolveLogFormat("")
	if err != nil || got != "short" {
		t.Fatalf("default: %q %v", got, err)
	}
	got, err = resolveLogFormat("LONG")
	if err != nil || got != "long" {
		t.Fatalf("long: %q %v", got, err)
	}
	if _, err := resolveLogFormat("medium"); err == nil {
		t.Fatal("medium should be rejected")
	}
}

func TestLoggerThresholdAndReset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "quiet.log")
	log := Logger{
		enabled: true,
		path:    path,
		locale:  "nb",
		format:  "long",
		now:     func() time.Time { return time.Date(2026, 10, 27, 17, 49, 0, 0, time.Local) },
	}
	ev := Event{
		Volume:    usage.Volume{Name: "pool", Where: "/pool", UsePct: 92, HasSize: true},
		Threshold: Threshold{Percent: 90, Script: "/tmp/clean.sh"},
	}
	if err := log.logThreshold(ev, true); err != nil {
		t.Fatal(err)
	}
	if err := log.logReset(Reset{Volume: ev.Volume, Percent: 90}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines=%v", lines)
	}
	if !strings.HasPrefix(lines[0], "27. oktober 2026 kl. 17:49:00 threshold /pool 90% (usage 92%); script /tmp/clean.sh") {
		t.Fatalf("fire line: %q", lines[0])
	}
	if !strings.Contains(lines[1], "reset /pool 90% (usage 92%)") {
		t.Fatalf("reset line: %q", lines[1])
	}

	noScript := Logger{
		enabled: true,
		path:    path,
		locale:  "en",
		format:  "short",
		now:     func() time.Time { return time.Date(2026, 10, 27, 18, 0, 0, 0, time.Local) },
	}
	if err := noScript.logThreshold(Event{
		Volume:    usage.Volume{Name: "root", Where: "/", UsePct: 81, HasSize: true},
		Threshold: Threshold{Percent: 80},
	}, false); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	last := strings.Split(strings.TrimSpace(string(data)), "\n")
	got := last[len(last)-1]
	if strings.Contains(got, "script") || !strings.HasSuffix(got, "threshold / 80% (usage 81%)") {
		t.Fatalf("no-script line: %q", got)
	}
}

func TestExpandLogPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := expandLogPath("~/omause/quiet.log")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(home, "omause/quiet.log") {
		t.Fatalf("got %q", got)
	}
}
