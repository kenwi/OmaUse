package quiet

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"

	"usage/internal/usage"
)

const notifyBin = "omarchy-notification-send"

// hardDiskGlyph is nf-md-harddisk. Omarchy's notification shell shows this
// via the omarchy-glyph hint from -g.
const hardDiskGlyph = "󰋊"

func notifyEvent(ev Event) error {
	pct := roundPct(ev.Volume)
	headline := "Disk usage high"
	body := fmt.Sprintf("%s is at %d%% (threshold %d%%)", displayName(ev.Volume.Name, ev.Volume.Where), pct, ev.Threshold.Percent)
	urgency := "normal"
	if ev.Threshold.Percent >= 90 {
		urgency = "critical"
	}
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
