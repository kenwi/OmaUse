package quiet

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"usage/internal/usage"
)

const notifyBin = "omarchy-notification-send"

// fluxBinNames are tried in order. flux-cli is the current command; flux is
// the older name, kept for builds that still install that binary.
var fluxBinNames = []string{"flux-cli", "flux"}

// hardDiskGlyph is nf-md-harddisk. Omarchy's notification shell shows this
// via the omarchy-glyph hint from -g.
const hardDiskGlyph = "󰋊"

// notifyEvents sends one desktop (and optional flux) alert covering every
// crossing in this quiet run, so same-title phones and shells do not hide
// all but the last toast.
func notifyEvents(events []Event, cfg Config) error {
	if len(events) == 0 {
		return nil
	}
	headline, body := formatAlerts(events)
	urgency := "normal"
	for _, ev := range events {
		if ev.Threshold.Percent >= 90 {
			urgency = "critical"
			break
		}
	}
	if err := desktopNotify(headline, body, urgency); err != nil {
		return err
	}
	if cfg.Flux.Enabled {
		fluxHeadline := headline
		if cfg.Flux.AnnounceHostname {
			if host, err := os.Hostname(); err == nil && host != "" {
				fluxHeadline = headline + " on " + host
			}
		}
		if err := fluxNotify(fluxHeadline, body); err != nil {
			return fmt.Errorf("flux: %w", err)
		}
	}
	return nil
}

func formatAlerts(events []Event) (headline, body string) {
	headline = "Disk usage high"
	if len(events) == 1 {
		ev := events[0]
		name := displayName(ev.Volume.Name, ev.Volume.Where)
		headline = fmt.Sprintf("Disk usage: %s", name)
		body = fmt.Sprintf("%s is at %d%% (threshold %d%%)", name, roundPct(ev.Volume), ev.Threshold.Percent)
		return headline, body
	}
	var lines []string
	for _, ev := range events {
		name := displayName(ev.Volume.Name, ev.Volume.Where)
		lines = append(lines, fmt.Sprintf("%s at %d%% (threshold %d%%)", name, roundPct(ev.Volume), ev.Threshold.Percent))
	}
	return headline, strings.Join(lines, "\n")
}

func desktopNotify(headline, body, urgency string) error {
	// Keep the default app name (omarchy-action). Omarchy's DND bypass only
	// allows omarchy-action and critical notify-send; a custom app name is
	// silenced when notification silencing is on.
	cmd := exec.Command(notifyBin,
		"-g", hardDiskGlyph,
		"-u", urgency,
		headline,
		body,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func fluxNotify(headline, body string) error {
	bin, err := resolveFluxBin()
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, "notify", headline, body)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// resolveFluxBin returns the first flux binary on PATH.
func resolveFluxBin() (string, error) {
	var tried []string
	for _, name := range fluxBinNames {
		path, err := exec.LookPath(name)
		if err == nil {
			return path, nil
		}
		tried = append(tried, name)
	}
	return "", fmt.Errorf("neither %s found in PATH", strings.Join(tried, " nor "))
}

func runScript(ev Event) error {
	script := ev.Threshold.Script
	if script == "" {
		return nil
	}
	cmd := exec.Command(script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"OMAUSE_NAME="+ev.Volume.Name,
		"OMAUSE_KIND="+ev.Volume.Kind,
		"OMAUSE_WHERE="+ev.Volume.Where,
		"OMAUSE_USE_PCT="+strconv.Itoa(roundPct(ev.Volume)),
		"OMAUSE_THRESHOLD="+strconv.Itoa(ev.Threshold.Percent),
		"OMAUSE_TOTAL="+strconv.FormatUint(ev.Volume.Total, 10),
		"OMAUSE_USED="+strconv.FormatUint(ev.Volume.Used, 10),
		"OMAUSE_FREE="+strconv.FormatUint(ev.Volume.Free, 10),
	)
	return cmd.Run()
}

func displayName(name, where string) string {
	if where != "" && where != "-" {
		if name != "" && name != where {
			return fmt.Sprintf("%s (%s)", name, where)
		}
		return where
	}
	if name != "" {
		return name
	}
	return "volume"
}

func roundPct(v usage.Volume) int {
	if !v.HasSize {
		return 0
	}
	return int(math.Round(v.UsePct))
}
