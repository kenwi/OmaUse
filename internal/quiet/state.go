package quiet

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

const stateFileName = "thresholds.json"

// State tracks which thresholds have already notified per volume.
// Keyed by volume identity (mount path, or pool:name).
type State struct {
	Fired map[string][]int `json:"fired"`
}

func stateDir() (string, error) {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "omause"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "omause"), nil
}

// StatePath is $XDG_STATE_HOME/omause/thresholds.json.
func StatePath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, stateFileName), nil
}

// LoadState reads persisted fired thresholds. Missing file yields empty state.
func LoadState() (State, error) {
	path, err := StatePath()
	if err != nil {
		return State{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{Fired: map[string][]int{}}, nil
		}
		return State{}, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, err
	}
	if st.Fired == nil {
		st.Fired = map[string][]int{}
	}
	for key, list := range st.Fired {
		st.Fired[key] = uniqueSorted(list)
	}
	return st, nil
}

// ClearState removes persisted fired thresholds so alerts can fire again.
// Missing state is not an error.
func ClearState() error {
	path, err := StatePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// SaveState writes fired thresholds atomically.
func SaveState(st State) error {
	dir, err := stateDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if st.Fired == nil {
		st.Fired = map[string][]int{}
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(dir, stateFileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func uniqueSorted(vals []int) []int {
	if len(vals) == 0 {
		return nil
	}
	seen := map[int]bool{}
	out := make([]int, 0, len(vals))
	for _, v := range vals {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}

func hasFired(list []int, pct int) bool {
	for _, v := range list {
		if v == pct {
			return true
		}
	}
	return false
}

func removeFired(list []int, pct int) []int {
	out := make([]int, 0, len(list))
	for _, v := range list {
		if v != pct {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func addFired(list []int, pct int) []int {
	if hasFired(list, pct) {
		return list
	}
	return uniqueSorted(append(list, pct))
}
