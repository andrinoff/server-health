package demo

import (
	"testing"
	"time"
)

func TestSnapshotLooksLikeAMachine(t *testing.T) {
	snap := Snapshot()

	if !snap.Supported || snap.Hostname != "demo-machine" {
		t.Fatalf("unexpected demo snapshot: %+v", snap)
	}
	if len(snap.Series) != samples {
		t.Errorf("series length = %d, want %d", len(snap.Series), samples)
	}
	span := snap.Series[len(snap.Series)-1].T - snap.Series[0].T
	if want := int64((samples - 1) * step / time.Millisecond); span != want {
		t.Errorf("series spans %d ms, want %d", span, want)
	}
	for i, point := range snap.Series {
		if point.CPU < 0 || point.CPU > 100 || point.Mem < 0 || point.Mem > 100 {
			t.Fatalf("point %d out of range: %+v", i, point)
		}
		if i > 0 && point.T <= snap.Series[i-1].T {
			t.Fatalf("series must advance in time at point %d", i)
		}
	}
	if snap.CPU != snap.Series[len(snap.Series)-1].CPU {
		t.Error("the headline reading should match the last sample")
	}
	if snap.MemUsed == 0 || snap.MemUsed > snap.MemTotal {
		t.Errorf("memory readings are inconsistent: %d of %d", snap.MemUsed, snap.MemTotal)
	}
	if snap.Disk.Pct <= 0 || snap.Disk.Pct >= 100 {
		t.Errorf("disk percentage = %v, want something in between", snap.Disk.Pct)
	}
	if snap.Uptime <= 0 {
		t.Errorf("uptime = %d, want positive", snap.Uptime)
	}
}

func TestServicesArePredictable(t *testing.T) {
	got := Services([]string{"home", "caddy.service", "ghost.service"})
	if len(got) != 3 {
		t.Fatalf("got %d units, want 3", len(got))
	}
	if got[0].Name != "home" || got[0].Unit != "home.service" {
		t.Errorf("first unit = %q/%q", got[0].Name, got[0].Unit)
	}
	if !got[0].Running() || !got[1].Running() {
		t.Error("the first two demo units should be running")
	}
	if got[2].Running() || got[2].MainPID != 0 || got[2].Since != "" {
		t.Errorf("the third demo unit should be down: %+v", got[2])
	}
}

func TestActionAcceptsOnlyRealActions(t *testing.T) {
	if err := Action("home.service", "restart", time.Second); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if err := Action("home.service", "explode", time.Second); err == nil {
		t.Fatal("expected an error for an unsupported action")
	}
}
