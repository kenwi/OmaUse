package quiet

import (
	"reflect"
	"testing"

	"usage/internal/usage"
)

func TestEvaluateFireAndReset(t *testing.T) {
	thresholds := []Threshold{
		{Percent: 80},
		{Percent: 90, Script: "/bin/true"},
	}
	vol := func(pct float64) usage.Volume {
		return usage.Volume{Name: "root", Where: "/", UsePct: pct, HasSize: true}
	}

	st := State{Fired: map[string][]int{}}

	// Cross 80%.
	events, next := Evaluate([]usage.Volume{vol(80)}, thresholds, st)
	if len(events) != 1 || events[0].Threshold.Percent != 80 {
		t.Fatalf("80%%: events=%v", events)
	}
	if len(next.Fired["/"]) != 0 {
		t.Fatalf("80%% should not mark until success, fired=%v", next.Fired["/"])
	}
	MarkFired(&next, "/", 80)
	st = next

	// Between thresholds: no new events, 80 stays fired.
	events, st = Evaluate([]usage.Volume{vol(85)}, thresholds, st)
	if len(events) != 0 {
		t.Fatalf("85%%: unexpected events %v", events)
	}
	if !reflect.DeepEqual(st.Fired["/"], []int{80}) {
		t.Fatalf("85%% fired=%v", st.Fired["/"])
	}

	// Cross 90%: notify + script once.
	events, next = Evaluate([]usage.Volume{vol(90)}, thresholds, st)
	if len(events) != 1 || events[0].Threshold.Percent != 90 || events[0].Threshold.Script == "" {
		t.Fatalf("90%%: events=%v", events)
	}
	if !reflect.DeepEqual(next.Fired["/"], []int{80}) {
		t.Fatalf("90%% kept prior only, fired=%v", next.Fired["/"])
	}
	MarkFired(&next, "/", 90)
	st = next

	// Still at 90%: nothing again.
	events, st = Evaluate([]usage.Volume{vol(92)}, thresholds, st)
	if len(events) != 0 {
		t.Fatalf("92%%: unexpected events %v", events)
	}

	// Drop below 80%: both reset.
	events, st = Evaluate([]usage.Volume{vol(50)}, thresholds, st)
	if len(events) != 0 {
		t.Fatalf("50%%: unexpected events %v", events)
	}
	if len(st.Fired["/"]) != 0 {
		t.Fatalf("50%% fired=%v, want reset", st.Fired["/"])
	}

	// Cross 80% again.
	events, next = Evaluate([]usage.Volume{vol(80)}, thresholds, st)
	if len(events) != 1 || events[0].Threshold.Percent != 80 {
		t.Fatalf("80%% again: events=%v", events)
	}
	MarkFired(&next, "/", 80)
	if !reflect.DeepEqual(next.Fired["/"], []int{80}) {
		t.Fatalf("80%% again fired=%v", next.Fired["/"])
	}
}

func TestEvaluateJumpPastBoth(t *testing.T) {
	thresholds := []Threshold{{Percent: 80}, {Percent: 90, Script: "cleanup"}}
	vol := usage.Volume{Name: "data", Where: "/data", UsePct: 95, HasSize: true}
	events, next := Evaluate([]usage.Volume{vol}, thresholds, State{})
	if len(events) != 2 {
		t.Fatalf("want both thresholds, got %v", events)
	}
	if events[0].Threshold.Percent != 80 || events[1].Threshold.Percent != 90 {
		t.Fatalf("order: %v", events)
	}
	if len(next.Fired["/data"]) != 0 {
		t.Fatalf("unmarked until success, fired=%v", next.Fired["/data"])
	}
	MarkFired(&next, "/data", 80)
	MarkFired(&next, "/data", 90)
	if !reflect.DeepEqual(next.Fired["/data"], []int{80, 90}) {
		t.Fatalf("fired=%v", next.Fired["/data"])
	}
}

func TestEvaluatePartialReset(t *testing.T) {
	thresholds := []Threshold{{Percent: 80}, {Percent: 90}}
	st := State{Fired: map[string][]int{"/": {80, 90}}}
	vol := usage.Volume{Name: "root", Where: "/", UsePct: 85, HasSize: true}
	events, next := Evaluate([]usage.Volume{vol}, thresholds, st)
	if len(events) != 0 {
		t.Fatalf("events=%v", events)
	}
	// 90 cleared, 80 remains.
	if !reflect.DeepEqual(next.Fired["/"], []int{80}) {
		t.Fatalf("fired=%v", next.Fired["/"])
	}
}

func TestParseConfig(t *testing.T) {
	cfg, err := parseConfig([]byte(`{
  "thresholds": [
    {"percent": 90, "script": "/tmp/clean.sh"},
    {"percent": 80},
    {"percent": 100, "drive": "/pool", "script": "/tmp/pool.sh"}
  ]
}`))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AlertsEnabled() {
		t.Fatal("omitted enabled should default on")
	}
	if len(cfg.Thresholds) != 3 {
		t.Fatalf("want 3 thresholds, got %+v", cfg.Thresholds)
	}
	if cfg.Thresholds[0].Percent != 80 || cfg.Thresholds[1].Percent != 90 || cfg.Thresholds[2].Percent != 100 {
		t.Fatalf("normalize order: %+v", cfg.Thresholds)
	}
	if cfg.Thresholds[2].Drive != "/pool" || cfg.Thresholds[2].Script != "/tmp/pool.sh" {
		t.Fatalf("drive threshold: %+v", cfg.Thresholds[2])
	}

	off, err := parseConfig([]byte(`{"enabled": false, "thresholds": [{"percent": 80}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if off.AlertsEnabled() {
		t.Fatal("enabled:false should disable alerts")
	}

	withFlux, err := parseConfig([]byte(`{"flux": true, "thresholds": [{"percent": 80}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !withFlux.Flux.Enabled || withFlux.Flux.AnnounceHostname {
		t.Fatalf("flux:true => %+v", withFlux.Flux)
	}

	withHost, err := parseConfig([]byte(`{"flux": {"enabled": true, "announce-hostname": true}, "thresholds": [{"percent": 80}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !withHost.Flux.Enabled || !withHost.Flux.AnnounceHostname {
		t.Fatalf("flux object => %+v", withHost.Flux)
	}
}

func TestEvaluateDriveSpecific(t *testing.T) {
	thresholds := []Threshold{
		{Percent: 80},
		{Percent: 90},
		{Percent: 100, Drive: "/pool", Script: "/tmp/pool.sh"},
	}
	root := usage.Volume{Name: "root", Where: "/", UsePct: 100, HasSize: true}
	pool := usage.Volume{Name: "pool", Where: "/pool", UsePct: 100, HasSize: true}

	events, _ := Evaluate([]usage.Volume{root, pool}, thresholds, State{})
	rootEvents := 0
	poolEvents := 0
	poolScript := false
	for _, ev := range events {
		switch VolumeKey(ev.Volume) {
		case "/":
			rootEvents++
			if ev.Threshold.Drive != "" {
				t.Fatalf("root should not get drive-specific event: %+v", ev.Threshold)
			}
		case "/pool":
			poolEvents++
			if ev.Threshold.Percent == 100 && ev.Threshold.Script != "" {
				poolScript = true
			}
		}
	}
	if rootEvents != 2 {
		t.Fatalf("root want global 80+90, got %d events", rootEvents)
	}
	if poolEvents != 3 {
		t.Fatalf("pool want 80+90+100, got %d events", poolEvents)
	}
	if !poolScript {
		t.Fatal("pool 100%% script event missing")
	}

	// Drive match by name also works.
	named := []Threshold{{Percent: 95, Drive: "pool"}}
	events, _ = Evaluate([]usage.Volume{pool}, named, State{})
	if len(events) != 1 {
		t.Fatalf("name match: events=%v", events)
	}
}

func TestVolumeKey(t *testing.T) {
	if got := VolumeKey(usage.Volume{Name: "root", Where: "/"}); got != "/" {
		t.Fatalf("got %q", got)
	}
	if got := VolumeKey(usage.Volume{Name: "tank", Where: "-", Kind: "zfs pool"}); got != "pool:tank" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatAlerts(t *testing.T) {
	one := []Event{{
		Volume:    usage.Volume{Name: "LaCie", Where: "/mnt/lacie", UsePct: 92, HasSize: true},
		Threshold: Threshold{Percent: 90},
	}}
	headline, body := formatAlerts(one)
	if headline != "Disk usage: LaCie (/mnt/lacie)" {
		t.Fatalf("single headline: %q", headline)
	}
	if body != "LaCie (/mnt/lacie) is at 92% (threshold 90%)" {
		t.Fatalf("single body: %q", body)
	}

	many := []Event{
		{Volume: usage.Volume{Name: "LaCie", Where: "/mnt/lacie", UsePct: 92, HasSize: true}, Threshold: Threshold{Percent: 80}},
		{Volume: usage.Volume{Name: "LaCie", Where: "/mnt/lacie", UsePct: 92, HasSize: true}, Threshold: Threshold{Percent: 90}},
		{Volume: usage.Volume{Name: "pool", Where: "/pool", UsePct: 87, HasSize: true}, Threshold: Threshold{Percent: 80}},
	}
	headline, body = formatAlerts(many)
	if headline != "Disk usage high" {
		t.Fatalf("multi headline: %q", headline)
	}
	want := "LaCie (/mnt/lacie) at 92% (threshold 80%)\nLaCie (/mnt/lacie) at 92% (threshold 90%)\npool (/pool) at 87% (threshold 80%)"
	if body != want {
		t.Fatalf("multi body:\n%s", body)
	}
}
