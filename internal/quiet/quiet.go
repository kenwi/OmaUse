package quiet

import (
	"fmt"
	"os"

	"usage/internal/usage"
)

// Run collects drive and pool usage once, applies threshold alerts from
// ~/.config/omause, then exits without opening the TUI.
func Run() error {
	snap := usage.Collect()
	cfg, ok, err := LoadConfig()
	if err != nil {
		return err
	}
	if !ok || !cfg.AlertsEnabled() || len(cfg.Thresholds) == 0 {
		return nil
	}
	st, err := LoadState()
	if err != nil {
		return err
	}
	events, next := Evaluate(snap.Volumes, cfg.Thresholds, st)
	for _, ev := range events {
		key := VolumeKey(ev.Volume)
		if err := notifyEvent(ev); err != nil {
			fmt.Fprintf(os.Stderr, "omause: notify %s at %d%%: %v\n", key, ev.Threshold.Percent, err)
			continue
		}
		if ev.Threshold.Script != "" {
			if err := runScript(ev); err != nil {
				fmt.Fprintf(os.Stderr, "omause: script %s for %s at %d%%: %v\n", ev.Threshold.Script, key, ev.Threshold.Percent, err)
				continue
			}
		}
		MarkFired(&next, key, ev.Threshold.Percent)
	}
	return SaveState(next)
}
