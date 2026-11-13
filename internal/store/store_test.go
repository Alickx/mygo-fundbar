package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFundsKeepAlertsAndTargets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "funds.json")
	want := &Data{
		Funds: []Entry{{
			Code: "007760", Shares: 12000, Cost: 1.1501,
			TakeProfit: 20, StopLoss: 8,
		}},
		Alerts: &Alerts{Move: true, MovePct: 2.5, Settled: true},
	}
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	got := Load(path)
	if got.Alerts == nil || *got.Alerts != *want.Alerts {
		t.Fatalf("alerts = %+v, want %+v", got.Alerts, want.Alerts)
	}
	if len(got.Funds) != 1 || got.Funds[0].TakeProfit != 20 || got.Funds[0].StopLoss != 8 {
		t.Fatalf("targets = %+v", got.Funds)
	}
}

// The low-coverage switch is kept with the funds, and a file from before it
// keeps the estimates off.
func TestLowCoverEstimateIsKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "funds.json")
	if err := (&Data{LowCoverEstimate: true}).Save(path); err != nil {
		t.Fatal(err)
	}
	if !Load(path).LowCoverEstimate {
		t.Fatal("the switch was not kept")
	}

	old := filepath.Join(t.TempDir(), "funds.json")
	if err := os.WriteFile(old, []byte(`{"funds":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if Load(old).LowCoverEstimate {
		t.Fatal("an older file turned the switch on by itself")
	}
}

func TestOlderFundsFileLoadsWithAlertsOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "funds.json")
	old := `{"funds":[{"code":"007760","shares":12000,"cost":1.1501}],"selected":"007760"}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load(path)
	if got.Alerts != nil {
		t.Fatalf("alerts = %+v, want none", got.Alerts)
	}
	if len(got.Funds) != 1 || got.Funds[0].TakeProfit != 0 || got.Funds[0].StopLoss != 0 {
		t.Fatalf("targets = %+v, want none", got.Funds)
	}
	// A newer build writes the added fields, and the old ones stay as they
	// were.
	got.Alerts = &Alerts{Move: true, MovePct: 2}
	got.Funds[0].TakeProfit = 20
	if err := got.Save(path); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["selected"]; !ok {
		t.Fatalf("the older fields were lost: %s", body)
	}
}

func TestAlertStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := &AlertState{
		Day: "2026-11-12", Move: []string{"007760", "008270"}, Settled: true,
		Targets: map[string]TargetMark{
			"007760": {Up: true, UpSent: true},
			"008270": {Down: true, DownSent: true},
		},
	}
	if err := SaveAlerts(dir, want); err != nil {
		t.Fatal(err)
	}
	got := LoadAlerts(dir)
	if got.Day != want.Day || got.Settled != want.Settled || len(got.Move) != 2 {
		t.Fatalf("state = %+v, want %+v", got, want)
	}
	if got.Targets["007760"] != want.Targets["007760"] ||
		got.Targets["008270"] != want.Targets["008270"] {
		t.Fatalf("targets = %+v, want %+v", got.Targets, want.Targets)
	}
}

func TestMissingAlertStateIsEmpty(t *testing.T) {
	got := LoadAlerts(t.TempDir())
	if got.Day != "" || got.Settled || len(got.Move) != 0 || len(got.Targets) != 0 {
		t.Fatalf("state = %+v, want empty", got)
	}
}

func TestManagerOnlyIsKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "funds.json")
	if err := (&Data{ManagerOnly: true}).Save(path); err != nil {
		t.Fatal(err)
	}
	if !Load(path).ManagerOnly {
		t.Fatal("the manager window was not kept")
	}
}
