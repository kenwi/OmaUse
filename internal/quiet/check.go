package quiet

import (
	"math"
	"strings"

	"usage/internal/usage"
)

// Event is one threshold crossing that should notify and maybe run a script.
type Event struct {
	Volume    usage.Volume
	Threshold Threshold
}

// Reset is a previously fired threshold that usage has fallen below.
type Reset struct {
	Volume  usage.Volume
	Percent int
}

// VolumeKey identifies a drive or pool across quiet runs.
func VolumeKey(v usage.Volume) string {
	if v.Where != "" && v.Where != "-" {
		return v.Where
	}
	return "pool:" + v.Name
}

// appliesTo is true for global thresholds, or when Drive matches this volume.
func (t Threshold) appliesTo(v usage.Volume) bool {
	if t.Drive == "" {
		return true
	}
	return driveMatches(t.Drive, v)
}

func driveMatches(drive string, v usage.Volume) bool {
	drive = normalizeDrive(drive)
	if drive == "" {
		return true
	}
	for _, cand := range []string{VolumeKey(v), v.Where, v.Name} {
		if cand == "" || cand == "-" {
			continue
		}
		if normalizeDrive(cand) == drive {
			return true
		}
		// pool:name keys also match a bare pool name.
		if strings.HasPrefix(cand, "pool:") && normalizeDrive(strings.TrimPrefix(cand, "pool:")) == drive {
			return true
		}
	}
	return false
}

// Evaluate compares current usage to configured thresholds.
// A threshold emits an event when use% reaches it and it is not already
// marked fired. When use% falls below that level, the fired mark is cleared
// and a Reset is returned.
// New crossings are not marked fired here - call MarkFired after notify/script
// succeed so a failed alert can retry on the next quiet run.
// Thresholds with Drive set only apply to that volume.
// Mounts matching ignore are skipped, and any fired mark for them is dropped.
func Evaluate(volumes []usage.Volume, thresholds []Threshold, ignore []string, st State) (events []Event, resets []Reset, next State) {
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
		if ignoredPath(vol.Where, ignore) {
			continue
		}
		pct := int(math.Round(vol.UsePct))
		prev := st.Fired[key]
		kept := make([]int, 0, len(prev))
		for _, t := range thresholds {
			if !t.appliesTo(vol) {
				continue
			}
			if pct >= t.Percent {
				if hasFired(prev, t.Percent) {
					kept = addFired(kept, t.Percent)
				} else {
					events = append(events, Event{Volume: vol, Threshold: t})
				}
				continue
			}
			if hasFired(prev, t.Percent) {
				resets = append(resets, Reset{Volume: vol, Percent: t.Percent})
			}
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
	return events, resets, next
}

// MarkFired records that a threshold was successfully handled for a volume.
func MarkFired(st *State, key string, percent int) {
	if st.Fired == nil {
		st.Fired = map[string][]int{}
	}
	st.Fired[key] = addFired(st.Fired[key], percent)
}
