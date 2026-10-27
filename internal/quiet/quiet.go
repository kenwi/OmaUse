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
	events, resets, next := Evaluate(snap.Volumes, cfg.Thresholds, cfg.Ignore, st)
	log := newLogger(cfg.Log)
	for _, r := range resets {
		if err := log.logReset(r); err != nil {
			fmt.Fprintf(os.Stderr, "omause: log: %v\n", err)
		}
	}
	if len(events) == 0 {
		return SaveState(next)
	}
	if err := notifyEvents(events, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "omause: notify: %v\n", err)
		return SaveState(next)
	}
	for _, ev := range events {
		key := VolumeKey(ev.Volume)
		scriptRan := false
		if ev.Threshold.Script != "" {
			if err := runScript(ev); err != nil {
				fmt.Fprintf(os.Stderr, "omause: script %s for %s at %d%%: %v\n", ev.Threshold.Script, key, ev.Threshold.Percent, err)
				if logErr := log.logThreshold(ev, false); logErr != nil {
					fmt.Fprintf(os.Stderr, "omause: log: %v\n", logErr)
				}
				continue
			}
			scriptRan = true
		}
		if err := log.logThreshold(ev, scriptRan); err != nil {
			fmt.Fprintf(os.Stderr, "omause: log: %v\n", err)
		}
		MarkFired(&next, key, ev.Threshold.Percent)
	}
	return SaveState(next)
}
