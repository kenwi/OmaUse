package quiet

import (
	"math"

	"usage/internal/usage"
)

// Event is one threshold crossing that should notify and maybe run a script.
type Event struct {
	Volume    usage.Volume
	Threshold Threshold
}

// VolumeKey identifies a drive or pool across quiet runs.
func VolumeKey(v usage.Volume) string {
	if v.Where != "" && v.Where != "-" {
		return v.Where
	}
	return "pool:" + v.Name
}

// Evaluate compares current usage to configured thresholds.
// A threshold emits an event when use% reaches it and it is not already
// marked fired. When use% falls below that level, the fired mark is cleared.
// New crossings are not marked fired here - call MarkFired after notify/script
// succeed so a failed alert can retry on the next quiet run.
func Evaluate(volumes []usage.Volume, thresholds []Threshold, st State) (events []Event, next State) {
	next = State{Fired: map[string][]int{}}
	if st.Fired == nil {
		st.Fired = map[string][]int{}
	}
	seen := map[string]bool{}
	for _, vol := range volumes {
		if !vol.HasSize {
			continue
		}
		key := VolumeKey(vol)
		seen[key] = true
		pct := int(math.Round(vol.UsePct))
		prev := st.Fired[key]
		kept := make([]int, 0, len(prev))
		for _, t := range thresholds {
			if pct >= t.Percent {
				if hasFired(prev, t.Percent) {
					kept = addFired(kept, t.Percent)
				} else {
					events = append(events, Event{Volume: vol, Threshold: t})
				}
				continue
			}
			// Below threshold: reset (omit from kept).
		}
		if len(kept) > 0 {
			next.Fired[key] = kept
		}
	}
	// Keep fired marks for volumes missing from this pass so a brief collect
	// glitch does not re-notify on the next successful run.
	for key, fired := range st.Fired {
		if seen[key] || len(fired) == 0 {
			continue
		}
		next.Fired[key] = uniqueSorted(append([]int(nil), fired...))
	}
	return events, next
}

// MarkFired records that a threshold was successfully handled for a volume.
func MarkFired(st *State, key string, percent int) {
	if st.Fired == nil {
		st.Fired = map[string][]int{}
	}
	st.Fired[key] = addFired(st.Fired[key], percent)
}
